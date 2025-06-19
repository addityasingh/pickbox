use log::info;
use tokio::net::TcpStream;
use tokio::io::{self, AsyncWriteExt, AsyncReadExt};

pub struct DFSClient {
    server_address: String,
}

impl DFSClient {
    pub fn new(server_address: &str) -> Self {
        info!("Creating DFSClient for server at {}", server_address);
        DFSClient {
            server_address: server_address.to_string(),
        }
    }

    pub async fn connect(&self) -> io::Result<TcpStream> {
        info!("Connecting to DFS server at {}", self.server_address);
        TcpStream::connect(&self.server_address).await
    }

    pub async fn send_request(&self, request: &str) -> io::Result<String> {
        let mut stream = self.connect().await?;
        info!("Sending request: {}", request);
        stream.write_all(request.as_bytes()).await?;

        let mut buffer = vec![0; 1024];
        let n = stream.read(&mut buffer).await?;
        let response = String::from_utf8_lossy(&buffer[..n]).to_string();
        info!("Received response: {}", response);
        Ok(response)
    }

    pub async fn open(&self, path: &str, mode: &str) -> io::Result<()> {
        info!("Opening file: {} with mode: {}", path, mode);
        let request = format!("OPEN {} {}", path, mode);
        self.send_request(&request).await?;
        Ok(())
    }

    pub async fn read(&self, path: &str, offset: u64, length: u64) -> io::Result<Vec<u8>> {
        info!("Reading file: {} from offset: {} with length: {}", path, offset, length);
        let request = format!("READ {} {} {}", path, offset, length);
        let response = self.send_request(&request).await?;
        Ok(response.into_bytes())
    }

    pub async fn write(&self, path: &str, offset: u64, data: &[u8]) -> io::Result<()> {
        info!("Writing to file: {} at offset: {} with data: {}", path, offset, String::from_utf8_lossy(data));
        let request = format!("WRITE {} {} {}", path, offset, String::from_utf8_lossy(data));
        self.send_request(&request).await?;
        Ok(())
    }

    pub async fn close(&self, path: &str) -> io::Result<()> {
        info!("Closing file: {}", path);
        let request = format!("CLOSE {}", path);
        self.send_request(&request).await?;
        Ok(())
    }
}

#[tokio::main]
async fn main() -> io::Result<()> {
    env_logger::init();
    let client = DFSClient::new("127.0.0.1:8085");
    client.open("data/example.txt", "r").await?;
    client.read("data/example.txt", 0, 100).await?;
    client.write("data/example.txt", 0, b"Hello, DFS!").await?;
    client.close("data/example.txt").await?;
    Ok(())
} 