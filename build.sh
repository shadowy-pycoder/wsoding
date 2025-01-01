#!/bin/sh

set -xe

mkdir -p build/
mkdir -p reports/

go build -o build/echo_client examples/echo_client/*.go
go build -o build/echo_server examples/echo_server/*.go
go build -o build/send_client examples/send_client/*.go
go build -o build/chat examples/chat/*.go