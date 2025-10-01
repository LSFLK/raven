# Silver Go IMAP Server

This is a IMAP server implementation in Go for Silver Mail. It supports basic IMAP functionalities and is designed to be lightweight and efficient. 

## How to run the server

1. Clone the repository:
   ```bash
   git clone https://github.com/Aravinda-HWK/Silver-IMAP.git
   cd Silver-IMAP
   ```

2. Build and run the Docker container:

```bash
docker build -t silver-imap .
docker run -it --rm -p 143:143 -p 993:993 silver-imap
```

3. The server will start and listen on ports 143 (IMAP) and 993 (IMAPS).
4. You can connect to the server using any IMAP client.

