use std::collections::{HashMap, HashSet};
use std::time::{SystemTime, UNIX_EPOCH};
use std::hash::{Hash, Hasher};
use std::collections::hash_map::DefaultHasher;

// Lamport timestamp for event ordering
#[derive(Clone, Debug, PartialEq, Eq, PartialOrd, Ord)]
struct LamportTimestamp {
    time: u64,
    node_id: NodeId,
}

// Operation-based CRDT for metadata
#[derive(Clone, Debug)]
enum Operation {
    CreateFile { path: String, id: FileId, timestamp: LamportTimestamp },
    WriteChunk { file_id: FileId, chunk_id: ChunkId, version: u64, timestamp: LamportTimestamp },
    DeleteFile { file_id: FileId, timestamp: LamportTimestamp },
    UpdateMetadata { file_id: FileId, key: String, value: String, timestamp: LamportTimestamp },
}

#[derive(Clone, Debug)]
struct OperationLog {
    operations: Vec<Operation>,
    seen_operations: HashSet<OperationId>,
}

impl OperationLog {
    fn new() -> Self {
        Self {
            operations: Vec::new(),
            seen_operations: HashSet::new(),
        }
    }
    
    fn apply(&mut self, op: Operation) -> bool {
        let op_id = self.compute_operation_id(&op);
        
        if self.seen_operations.contains(&op_id) {
            return false; // Already applied
        }
        
        self.operations.push(op.clone());
        self.seen_operations.insert(op_id);
        true
    }
    
    fn merge(&mut self, other: &OperationLog) -> Vec<Operation> {
        let mut new_ops = Vec::new();
        
        for op in &other.operations {
            if self.apply(op.clone()) {
                new_ops.push(op.clone());
            }
        }
        
        new_ops
    }
    
    fn compute_operation_id(&self, op: &Operation) -> OperationId {
        let mut hasher = DefaultHasher::new();

        // Hash the operation variant and its contents
        match op {
            Operation::CreateFile { path, id, timestamp } => {
                "CreateFile".hash(&mut hasher);
                path.hash(&mut hasher);
                id.hash(&mut hasher);
                timestamp.hash(&mut hasher);
            },
            Operation::WriteChunk { file_id, chunk_id, version, timestamp } => {
                "WriteChunk".hash(&mut hasher);
                file_id.hash(&mut hasher);
                chunk_id.hash(&mut hasher);
                version.hash(&mut hasher);
                timestamp.hash(&mut hasher);
            },
            Operation::DeleteFile { file_id, timestamp } => {
                "DeleteFile".hash(&mut hasher);
                file_id.hash(&mut hasher);
                timestamp.hash(&mut hasher);
            },
            Operation::UpdateMetadata { file_id, key, value, timestamp } => {
                "UpdateMetadata".hash(&mut hasher);
                file_id.hash(&mut hasher);
                key.hash(&mut hasher);
                value.hash(&mut hasher);
                timestamp.hash(&mut hasher);
            },
        }

        hasher.finish()
    }
}

// State-based CRDT for directory structure
#[derive(Clone, Debug)]
struct DirectoryCRDT {
    entries: HashMap<String, DirectoryEntry>,
    tombstones: HashSet<String>,
    vector_clock: HashMap<NodeId, u64>,
}

#[derive(Clone, Debug)]
enum DirectoryEntry {
    File(FileId),
    Directory(DirectoryCRDT),
}

impl DirectoryCRDT {
    fn new() -> Self {
        Self {
            entries: HashMap::new(),
            tombstones: HashSet::new(),
            vector_clock: HashMap::new(),
        }
    }
    
    fn add_entry(&mut self, path: &str, entry: DirectoryEntry, node_id: NodeId) {
        if self.tombstones.contains(path) {
            return; // Entry was deleted
        }
        
        self.entries.insert(path.to_string(), entry);
        self.increment_clock(node_id);
    }
    
    fn remove_entry(&mut self, path: &str, node_id: NodeId) {
        self.entries.remove(path);
        self.tombstones.insert(path.to_string());
        self.increment_clock(node_id);
    }
    
    fn merge(&mut self, other: &DirectoryCRDT) -> bool {
        let mut changed = false;
        
        // Merge tombstones (deleted entries)
        for path in &other.tombstones {
            if !self.tombstones.contains(path) {
                self.tombstones.insert(path.clone());
                self.entries.remove(path);
                changed = true;
            }
        }
        
        // Merge entries
        for (path, entry) in &other.entries {
            if !self.tombstones.contains(path) {
                match (self.entries.get(path), entry) {
                    (None, _) => {
                        // Entry doesn't exist locally, add it
                        self.entries.insert(path.clone(), entry.clone());
                        changed = true;
                    }
                    (Some(DirectoryEntry::Directory(local_dir)), DirectoryEntry::Directory(remote_dir)) => {
                        // Recursively merge directories
                        let mut local_dir_clone = local_dir.clone();
                        if local_dir_clone.merge(remote_dir) {
                            self.entries.insert(path.clone(), DirectoryEntry::Directory(local_dir_clone));
                            changed = true;
                        }
                    }
                    (Some(DirectoryEntry::File(_)), DirectoryEntry::File(_)) => {
                        // For files, we need to check vector clocks to determine which version wins
                        if self.compare_vector_clocks(&other.vector_clock) == Ordering::Less {
                            self.entries.insert(path.clone(), entry.clone());
                            changed = true;
                        }
                    }
                    _ => {
                        // Type conflict (file vs directory) - use the entry with the higher vector clock
                        if self.compare_vector_clocks(&other.vector_clock) == Ordering::Less {
                            self.entries.insert(path.clone(), entry.clone());
                            changed = true;
                        }
                    }
                }
            }
        }
        
        // Merge vector clocks
        self.merge_vector_clocks(&other.vector_clock);
        
        changed
    }
    
    fn increment_clock(&mut self, node_id: NodeId) {
        let current = self.vector_clock.entry(node_id).or_insert(0);
        *current += 1;
    }
    
    fn merge_vector_clocks(&mut self, other: &HashMap<NodeId, u64>) {
        for (node_id, &timestamp) in other {
            let current = self.vector_clock.entry(*node_id).or_insert(0);
            *current = std::cmp::max(*current, timestamp);
        }
    }
    
    fn compare_vector_clocks(&self, other: &HashMap<NodeId, u64>) -> std::cmp::Ordering {
        // Implement vector clock comparison
        // Returns Less if other is newer, Greater if self is newer, Equal if concurrent
        for (node_id, &timestamp) in other {
            let current = self.vector_clock.entry(*node_id).or_insert(0);
            if *current < timestamp {
                return std::cmp::Ordering::Less;
            }
        }
        std::cmp::Ordering::Greater

    }
}

pub struct ConsensusService {
    nodes: HashMap<NodeId, Node>,
    operation_logs: HashMap<NodeId, OperationLog>,
    directory_crdt: DirectoryCRDT,
}

impl ConsensusService {
    pub fn new() -> Self {
        ConsensusService { nodes: HashMap::new(), operation_logs: HashMap::new(), directory_crdt: DirectoryCRDT::new() }
    }

}   