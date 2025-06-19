use crate::pickbox::storage_manager::{ChunkId, NodeId, FileId};
use tokio::sync::mpsc;
use log::info;

// Define a struct to represent a job
pub struct ReplicationJob {
    pub chunk_id: ChunkId,
    pub target_node: NodeId,
}

// Define the process_replication function
async fn process_replication(replication_job: ReplicationJob) {
    info!("Processing replication job for chunk: {:?}", replication_job.chunk_id);
}

// Define the process_client_request function
async fn process_client_request(socket: tokio::net::TcpStream, tx: mpsc::Sender<ReplicationJob>) {
    info!("Processing client request");
    let replication_job = ReplicationJob {
        chunk_id: ChunkId {
            file_id: FileId { file_id: 1 },
            chunk_index: 0,
        },
        target_node: NodeId { node_id: 2 },
    };

    tx.send(replication_job).await.unwrap();
}
