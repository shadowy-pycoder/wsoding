
# wsoding - [Tsoding's](https://github.com/tsoding) [c3ws](https://github.com/tsoding/c3ws) library ported to Go


Work in progress


```shell
docker run -it --rm \               
    -v ${PWD}/config:/config \
    -v ${PWD}/reports:/reports \
    crossbario/autobahn-testsuite \
    wstest -m fuzzingclient -s /config/fuzzingclient.json
```