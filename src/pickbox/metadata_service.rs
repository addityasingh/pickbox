// Metadata service: Track the location of file blocks

use std::collections::HashMap;
use std::time::{SystemTime, UNIX_EPOCH};
use std::hash::{Hash, Hasher};
use std::collections::hash_map::DefaultHasher;


pub struct MetadataService {
    files: HashMap<FileId, FileMetadata>,
    directory_tree: DirectoryTree,
    node_registry: HashMap<NodeId, NodeStatus>,
    storage_nodes: Vec<StorageNode>,
}

pub struct FileMetadata {
    file_id: FileId,
    file_name: String,
    size: u64,
    chunk_map: Vec<ChunkPlacement>,
    permissions: FilePermissions,
    created_at: DateTime<Utc>,
    modified_at: DateTime<Utc>,
    version: u64,
}

pub struct ChunkPlacement {
    chunk_id: ChunkId,
    primary_node: NodeId,
    replicas: Vec<NodeId>,
    state: ReplicationState,
}

pub enum ReplicationState {
    Pending,
    Replicated,
    Failed,
}

pub struct FileId {
    file_name: String,
}

impl MetadataService {
    pub fn new() -> Self {
        MetadataService { 
            files: HashMap::new(), 
            directory_tree: DirectoryTree::new(), 
            node_registry: HashMap::new(), 
            storage_nodes: Vec::new() 
        }
    }
    
    pub fn register_node(&mut self, node_id: NodeId) {
        self.node_registry.insert(node_id, NodeStatus::new());
    }
    
    async fn create_file(&mut self, file_name: String, size: u64) -> Result<FileId, MetadataError> {
        let file_id = FileId::new(file_name);
        let file_metadata = FileMetadata::new(file_id, size);
        self.files.insert(file_id, file_metadata);
        Ok(file_id)
    }
    
    async fn get_chunk_locations(&self, file_id: FileId, chunk_id: ChunkId) -> Result<Vec<NodeId>, MetadataError> {
        let file_metadata = self.files.get(&file_id).unwrap();
        let chunk_placement = file_metadata.chunk_map.iter().find(|placement| placement.chunk_id == chunk_id).unwrap();
        Ok(chunk_placement.replicas.clone())
    }

    async fn update_file_metadata(&mut self, file_id: FileId, metadata: FileMetadata) -> Result<(), MetadataError> {
        self.files.insert(file_id, metadata);
        Ok(())
    }
    
}   

