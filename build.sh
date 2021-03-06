#!/bin/bash

go get -d ./...
go test .
env GOOS=linux GOARCH=amd64 go build -o helm-ssm-Linux-x86_64 main.go
env GOOS=darwin GOARCH=amd64 go build -o helm-ssm-Darwin-x86_64 main.go
env GOOS=darwin GOARCH=arm64 go build -o helm-ssm-Darwin-arm64 main.go
