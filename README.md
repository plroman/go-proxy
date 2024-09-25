# orderflow-proxy

[![Goreport status](https://goreportcard.com/badge/github.com/flashbots/orderflow-proxy)](https://goreportcard.com/report/github.com/flashbots/go-template)
[![Test status](https://github.com/flashbots/orderflow-proxy/actions/workflows/checks.yml/badge.svg?branch=main)](https://github.com/flashbots/go-template/actions?query=workflow%3A%22Checks%22)

## Getting started

**Build**

```bash
make build
```

## Run

`./build/orderflow-proxy`

Will 

* create metrics server
* internal server (liveness endpoints)
* orderflow proxy server that will generate self signed certificate and use it to accept requests that will be proxied to the local builder endpoint

Flags for the orderflow proxy

```
--listen-addr value                        address to listen on for orderflow proxy API (default: "127.0.0.1:9090")
--builder-endpoint value                   address to send local ordeflow to (default: "127.0.0.1:8546")
--cert-duration value                      generated certificate duration (default: 8760h0m0s)
--cert-hosts value [ --cert-hosts value ]  generated certificate hosts (default: "127.0.0.1", "localhost")
```


## curl TLS example

1. Run orderflow proxy

```bash
make build 
./build/orderflow-proxy --listen-addr "localhost:8000"
```

2. Extract self signed certificate 
```bash
# -k will tell curl to ignore the fact that cert is self signed
curl -w %{certs} -k https://localhost:8000 > cacert.pem
```
3. Make call using this certificate
```bash
curl https://localhost:8000 --cacert cacert.pem
```
