use pickbox::storage_manager::{StorageNode, NodeId};
use pickbox::replication_manager::ReplicationJob;
use tokio::net::{TcpListener, TcpStream};
use tokio::sync::mpsc;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use log::{info, error};
use std::fs::{OpenOptions, File};
use std::io::{self, Read, Write, Seek, SeekFrom};
use std::path::Path;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Initialize the logger
    env_logger::init();

    // Initialize multiple storage nodes
    let mut storage_nodes = Vec::new();
    for i in 0..3 {
        let node_id = NodeId { node_id: i };
        let storage_node = StorageNode::new(node_id).await?;
        storage_nodes.push(storage_node);
        info!("Initialized storage node with ID: {}", i);
    }
    
    // Use channels for inter-thread communication
    let (tx, mut rx) = mpsc::channel::<ReplicationJob>(100);
    
    // Spawn a background task for handling chunk replication
    tokio::spawn(async move {
        while let Some(replication_job) = rx.recv().await {
            // Process replication in background
            process_replication(replication_job).await;
        }
    });
    
    // Handle client requests
    let listener = TcpListener::bind("127.0.0.1:8085").await?;
    info!("Server listening on 127.0.0.1:8085");

    loop {
        let (socket, _) = listener.accept().await?;
        let tx_clone = tx.clone();
        
        // Process each client connection in its own task
        tokio::spawn(async move {
            process_client_request(socket, tx_clone).await;
        });
    }
}

// Define the process_replication function
async fn process_replication(replication_job: ReplicationJob) {
    info!("Processing replication job for chunk: {:?}", replication_job.chunk_id);
}

// Define the process_client_request function
async fn process_client_request(mut socket: TcpStream, tx: mpsc::Sender<ReplicationJob>) {
    info!("Processing client request");
    let mut buffer = [0; 1024];
    match socket.read(&mut buffer).await {
        Ok(n) if n == 0 => return, // Connection was closed
        Ok(n) => {
            let request = String::from_utf8_lossy(&buffer[..n]);
            info!("Received request: {}", request);

            let response = match request.split_whitespace().collect::<Vec<&str>>().as_slice() {
                ["OPEN", path, mode] => {
                    // Handle open operation
                    let file_path = Path::new(path);
                    let file = match *mode {
                        "r" => OpenOptions::new().read(true).open(file_path),
                        "w" => OpenOptions::new().write(true).create(true).open(file_path),
                        "a" => OpenOptions::new().append(true).open(file_path),
                        _ => Err(io::Error::new(io::ErrorKind::InvalidInput, "Invalid mode")),
                    };

                    match file {
                        Ok(_) => {
                            info!("Opened file: {} with mode: {}", path, mode);
                            "File opened".to_string()
                        }
                        Err(e) => {
                            error!("Failed to open file: {} with mode: {}: {}", path, mode, e);
                            format!("Failed to open file: {}", e)
                        }
                    }
                }
                ["READ", path, offset, length] => {
                    // Handle read operation
                    let file_path = Path::new(path);
                    let mut file = match File::open(file_path) {
                        Ok(file) => file,
                        Err(e) => {
                            error!("Failed to open file for reading: {}", e);
                            return;
                        }
                    };

                    let offset: u64 = offset.parse().unwrap_or(0);
                    let length: usize = length.parse().unwrap_or(0);
                    let mut buffer = vec![0; length];

                    if let Err(e) = file.seek(SeekFrom::Start(offset)) {
                        error!("Failed to seek in file: {}", e);
                        return;
                    }

                    match file.read(&mut buffer) {
                        Ok(_) => {
                            let data = String::from_utf8_lossy(&buffer);
                            info!("Read data from file: {}", data);
                            data.to_string()
                        }
                        Err(e) => {
                            error!("Failed to read file: {}", e);
                            format!("Failed to read file: {}", e)
                        }
                    }
                }
                ["WRITE", path, offset, data] => {
                    // Handle write operation
                    let file_path = Path::new(path);
                    let mut file = match OpenOptions::new().write(true).open(file_path) {
                        Ok(file) => file,
                        Err(e) => {
                            error!("Failed to open file for writing: {}", e);
                            return;
                        }
                    };

                    let offset: u64 = offset.parse().unwrap_or(0);
                    if let Err(e) = file.seek(SeekFrom::Start(offset)) {
                        error!("Failed to seek in file: {}", e);
                        return;
                    }

                    match file.write_all(data.as_bytes()) {
                        Ok(_) => {
                            info!("Wrote data to file: {}", data);
                            "Write successful".to_string()
                        }
                        Err(e) => {
                            error!("Failed to write to file: {}", e);
                            format!("Failed to write to file: {}", e)
                        }
                    }
                }
                ["CLOSE", path] => {
                    // Handle close operation
                    info!("Closing file: {}", path);
                    "File closed".to_string()
                }
                _ => "Invalid command".to_string(),
            };

            if let Err(e) = socket.write_all(response.as_bytes()).await {
                error!("Failed to send response: {}", e);
            }
        }
        Err(e) => {
            error!("Failed to read from socket: {}", e);
        }
    }
} 