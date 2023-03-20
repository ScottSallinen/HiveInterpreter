## HiveInterpreter

An interpretation layer to sit between nginx and hive API server(s).
Takes incoming requests, determines whether they need history or hivemind, and directs appropriately.
This does not perform methods like load balancing, this is handled by nginx after the incoming request has been normalized.
Performs some simple caching on the normalized request.

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
