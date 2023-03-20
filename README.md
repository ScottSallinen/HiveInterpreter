## HiveInterpreter

An interpretation layer to sit between nginx and hive API server(s).
Takes incoming requests, determines whether they need history or hivemind, and directs appropriately.
This does not perform methods like load balancing, this is handled by nginx after the incoming request has been normalized.
Performs some simple caching on the normalized request.


### REST Interpretation

The layer also provides a simple interpretation of REST calls to the API servers. For example, you can use a simple call such as
```http://anyx.io/v1/database_api/get_dynamic_global_properties```

Methods with arguments are flattened, e.g.:
```
curl -s -d '{"jsonrpc":"2.0", "method":"block_api.get_block", "params":{"block_num":60000000}, "id":"0"}' https://anyx.io
```
Can be instead used as:
```
http://anyx.io/v1/block_api/get_block?block_num=60000000
```
And so on.

A few extension APIs are provided from this interface, such as `get_block_by_time`.
```http://anyx.io/v1/block_api/get_block_by_time?timestamp=2021-12-13T11:30:36```

### Install
Install go. Suggested method:
```
mkdir -p ~/go
cd ~/go
git clone https://github.com/udhos/update-golang
cd update-golang
sudo ./update-golang.sh
```

Clone this repo and build/install the framework. e.g.,
```
cd ~
git clone https://github.com/ScottSallinen/HiveInterpreter.git
cd HiveInterpreter
go install ./...
```

### Set up nginx
This interpreter sits between an input nginx, a second nginx interface, and the hive API server(s).
The flow for an API request is as follows:

``` nginx(default) -> HiveInterpreter -> nginx(nginx2upstream) -> hive API server(s)```

Because these connections all use unix sockets, it is still fast despite using nginx as a proxy.
This also means you can customize nginx2upstream to be your load balancer in a standard way.
Simply customize the upstream directives as needed.
Use the files provided in nginxExample as a starting point to set up nginx.


### Launch the Interpreter
With the example provided nginx, the interpreter should be launched as follows:
 - Listening on the unix socket that nginx provides in default
 - Connecting to the nginx unix sockets from nginx2upstream:
    - For lite requests
    - For full (history) requests
    - For hivemind requests

Example:
```
hiveInterpreter -l "/dev/shm/hiveinterpreter.sock" -c "unix:/dev/shm/nginxToLite.sock" -f "unix:/dev/shm/nginxToFull.sock" -h "unix:/dev/shm/nginxToHivemind.sock"
```

Some additional options:
```
-d : Enable debug logging.
-w : Worker threads (size of the worker pool).
-q : Per worker queue size per upstream.
-p : Optional separate endpoint for push transaction. Need to add another upstream to nginx if this is used.
```
