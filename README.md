
# wsoding - [Tsoding's](https://github.com/tsoding) [c3ws](https://github.com/tsoding/c3ws) library translated to Go


> *Made it work for majority of the Autobahn Test Cases, so this project can be considered done (with extremely unfriendly API)*

## Echo Server

```bash
./build.sh 
```

In one terminal:
```shell
./build/echo_server
```

In another terminal
```shell
./build/send_client 127.0.0.1 9001 "Hello, World" 
```

You can also connect to the server from a browser:
```shell
firefox ./tools/example_send_client.html
```

## Autobahn Test Suite

```shell
docker run -it --rm --net=host\               
    -v ${PWD}/autobahn:/config \
    -v ${PWD}/reports:/reports \
    crossbario/autobahn-testsuite \
    wstest -m fuzzingclient -s /config/fuzzingclient.json
```

```shell
firefox ./reports/servers/index.html
```