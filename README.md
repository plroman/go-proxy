# orderflow-proxy

[![Goreport status](https://goreportcard.com/badge/github.com/flashbots/tdx-orderflow-proxy)](https://goreportcard.com/report/github.com/flashbots/go-template)
[![Test status](https://github.com/flashbots/tdx-orderflow-proxy/actions/workflows/checks.yml/badge.svg?branch=main)](https://github.com/flashbots/go-template/actions?query=workflow%3A%22Checks%22)

## Getting started

**Build**

```bash
make build
```

## Run

Orderflow proxy will: 

* generate SSL certificate
* generate orderflow signer
* create 2 input servers serving TLS with that certificate (user-listen-addr, network-listen-addr)
* create 1 local http server serving /cert  (cert-listen-addr)
* create metrics server (metrict-addr)
* proxy requests to local builder (from user and network/users listen addresses to the builder-endpoint)
* proxy user request to other builders in the network
* archive user requests by sending them to archive endpoint

Flags for the orderflow proxy

```
./build/orderflow-proxy -h 
NAME:
   orderflow-proxy - Serve API, and metrics

USAGE:
   orderflow-proxy [global options] command [command options] 

COMMANDS:
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --users-listen-addr value                   address to listen on for orderflow proxy API for external users and local operator (default: "127.0.0.1:443")
   --network-listen-addr value                 address to listen on for orderflow proxy API for other network participants (default: "127.0.0.1:5544")
   --cert-listen-addr value                    address to listen on for orderflow proxy serving its SSL certificate on /cert (default: "127.0.0.1:14727")
   --builder-endpoint value                    address to send local ordeflow to (default: "http://127.0.0.1:8645")
   --rpc-endpoint value                        address of the node RPC that supports eth_blockNumber (default: "http://127.0.0.1:8545")
   --builder-confighub-endpoint value          address of the builder config hub enpoint (directly or throught the cvm-proxy) (default: "http://127.0.0.1:14892")
   --orderflow-archive-endpoint value          address of the ordreflow archive endpoint (block-processor) (default: "http://127.0.0.1:14893")
   --builder-name value                        name of this builder (same as in confighub) (default: "test-builder")
   --flashbots-orderflow-signer-address value  ordreflow from Flashbots will be signed with this address (default: "0x5015Fa72E34f75A9eC64f44a4Fcf0837919D1bB7")
   --cert-duration value                       generated certificate duration (default: 8760h0m0s)
   --cert-hosts value [ --cert-hosts value ]   generated certificate hosts (default: "127.0.0.1", "localhost")
   --metrics-addr value                        address to listen on for Prometheus metrics (metrics are served on $metrics-addr/metrics) (default: "127.0.0.1:8090")
   --log-json                                  log in JSON format (default: false)
   --log-debug                                 log debug messages (default: false)
   --log-uid                                   generate a uuid and add to all log messages (default: false)
   --log-service value                         add 'service' tag to logs (default: "your-project")
   --pprof                                     enable pprof debug endpoint (pprof is served on $metrics-addr/debug/pprof/*) (default: false)
   --help, -h                                  show help
```
