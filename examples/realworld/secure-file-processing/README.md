# Secure File Processing Example

Demonstrates secure upload handling and durable background file processing.

## Features

- security headers
- body size limit
- multipart upload
- file extension validation
- durable `document.scan` queue job
- static downloads for accepted files

## Run

```bash
go run . -addr :3000
```

## Test

```bash
echo 'hello' > sample.txt
curl -i -X POST http://localhost:3000/documents \
  -H 'Idempotency-Key: doc-001' \
  -F 'file=@sample.txt'
```
