// Storage manager: Manage physical storage of file blocks and replication
// 1. Receive file upload requests from the client
// 2. Store the file blocks on the storage nodes
// 3. Replicate the file blocks across multiple nodes
// 4. Return a success message to the client
use std::collections::HashMap;

pub struct NodeId {
    pub node_id: u32,
}

#[derive(Eq, PartialEq, Hash, Debug)]
pub struct FileId {
    pub file_id: u32,
}

pub struct StorageManager {
    storage_nodes: Vec<StorageNode>,
}

pub struct StorageNode {
    id: NodeId,
    chunks: HashMap<ChunkId, DataChunk>,
    chunk_roles: HashMap<ChunkId, ChunkRole>,  
}

pub struct DataChunk {
    id: ChunkId,
    data: Vec<u8>,
    checksum: u64,
    version: u64,
}

#[derive(Debug, Eq, Hash, PartialEq)]
pub struct ChunkId {
    pub file_id: FileId,
    pub chunk_index: u32,
}

#[derive(Eq, Hash, PartialEq)]
pub enum ChunkRole {
    Primary,
    Replica,
}

// Implemement conflict resolution with vector clocks  
pub struct VectorClock {
    timestamps: HashMap<NodeId, u64>,
}

impl VectorClock {
    pub fn new() -> Self {
        VectorClock { timestamps: HashMap::new() }
    }
}

impl NodeId {
    pub fn new() -> Self {
        NodeId {
            node_id: 0, // or any default value
        }
    }
}

impl FileId {
    pub fn new() -> Self {
        FileId {
            file_id: 0, // or any default value
        }
    }
}

impl StorageNode {
    pub async fn new(id: NodeId) -> Result<Self, Box<dyn std::error::Error>> {
        // Initialize fields with appropriate values
        Ok(StorageNode {
            id,
            chunks: HashMap::new(),
            chunk_roles: HashMap::new(),
        })
    }
   
    async fn store_chunk(&mut self, chunk_id: ChunkId, role: ChunkRole) {
        self.chunk_roles.insert(chunk_id, role);
    }

    async fn retrieve_chunk(&self, chunk_id: ChunkId) -> Option<&DataChunk> {
        self.chunks.get(&chunk_id)
    }

    async fn replicate_chunk(&self, chunk_id: ChunkId, target_node: NodeId) {
        let role = self.chunk_roles.get(&chunk_id).unwrap();
        if role == &ChunkRole::Primary {
            // TODO: Replicate the chunk to the other nodes
            println!("Replicating chunk to target node: {}", target_node.node_id);
        }
    }
}
