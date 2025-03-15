# Ignite Provision API

A Go-based REST API service for provisioning and managing virtual machines using Weave Ignite.

## Overview

This service provides HTTP endpoints to:
- Provision master nodes
- Provision worker nodes
- Delete VMs
- Health check

## Prerequisites

- Go 1.16 or later
- [Weave Ignite](https://github.com/weaveworks/ignite) installed
- `sudo` access for running Ignite commands
- Docker (for running the container images)

## Project Structure 

```
ignite-api
├── cmd
│ └── main.go # Application entry point
├── internal
│ ├── api
│ │ └── handlers.go # HTTP request handlers
│ ├── config
│ │ └── types.go # Configuration types
│ ├── models
│ │ └── models.go # Data models
│ └── utils
│ └── ignite.go # Utility functions for Ignite operations
└── README.md
```


## API Endpoints

1. Health Check
2. Provision Master Node
3. Provision Worker Node
4. Delete VM

### 1. Health Check

```
GET /health
```
Response:
```json
{
  "status": "ok",
  "service": "ignite-provision-api"
}
``` 

### 2. Provision Master Node

```
POST /master/provision
```

Request Body:

```json
{
"nodeName": "master-1",
"nodeUid": "unique-id",
"nodeType": "master",
"token": "your-token",
"cpus": 2,
"diskSize": "3GB",
"memory": "1GB",
"imageOci": "shajalahamedcse/only-k3-go:v1.0.10",
"enableSsh": true
}
```

Response:
```json
{
  "success": true,
  "message": "Master node provisioned successfully",
  "masterIP": "10.62.0.191"
}
```

### 3. Provision Worker Node

```
POST /worker/provision
```

Request Body:

```json
{
"nodeName": "worker-1",
"nodeUid": "unique-id",
"nodeType": "worker",
"token": "your-token",
"masterIP": "master-node-ip",
"cpus": 2,
"diskSize": "3GB",
"memory": "1GB",
"imageOci": "shajalahamedcse/only-k3-go:v1.0.10",
"enableSsh": true
}
```

Response:
```json
{
  "success": true,
  "message": "Worker node provisioned successfully",
  "nodeId": "worker-node-id"
}
```

### 4. Delete VM

Request Body:

```
DELETE /vm/master-1
``` 

Response:
```json
{
  "success": true,
  "message": "VM deleted successfully"
}
```     


## Configuration

The service stores VM information in a CSV file (`provisioned_vms.csv`) with the following columns:
- NodeName
- NodeUID
- MasterIP
- NodeType
- Token

## Running the Service

1. Clone the repository:

```bash
git clone https://github.com/Ridwan414/ignite-api.git
cd ignite-api
```

2. Build the binary:

```bash
go build -o ignite-api cmd/main.go
```

3. Run the service:

```bash
./ignite-api
``` 

## Security Considerations

- The service requires sudo access for Ignite operations
- Token validation is implemented for worker node provisioning
- Temporary files are properly cleaned up after use
