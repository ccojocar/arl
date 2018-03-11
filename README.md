# arl

This is a tool to measure the rate-limit of REST API protected by Azure Active Directory

## Installation

```bash
go get github.com/cosmincojocar/arl
```

## Usage 

```bash
$ arl -h
Usage of ./arl:
  -client-id string
        client ID
  -num-tokens int
        number of tokens requested for a user (default 1)
  -parallel-reqs int
        number of parallel request (default 8)
  -resource string
        REST resource for which the rate limit measurement is executed
  -tenant-id string
        tenant ID
```

The API rate-limit for a REST resource can be measured as follows:

```bash
$ arl -resouce <RESSOURCE_URL> -client-id <AAD_CLIENT_ID> -tenant-id <AAD_TENANT_ID>
```

The tool will prompt a device code which can be used to authenticate with AAD.
