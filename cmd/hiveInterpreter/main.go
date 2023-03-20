package main

import (
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/patrickmn/go-cache"
)

// Custom jsoniter config. Useful for sorting.
var jsonit = jsoniter.Config{
	EscapeHTML:              false,
	MarshalFloatWith6Digits: false,
	DisallowUnknownFields:   false,
	OnlyTaggedField:         false,
	ValidateJsonRawMessage:  false,
	CaseSensitive:           true,
	UseNumber:               true,
	SortMapKeys:             true,
}.Froze()

var debug bool
var respcache *cache.Cache
var workers int

var fullep string
var liteep string
var hiveep string
var pushep string

var ep2pool map[string]jobPool

func main() {
	dptr := flag.Bool("d", false, "Debug mode")
	wptr := flag.Int("w", 64, "Worker threads")
	qptr := flag.Int("q", 8, "Per worker queue size per upstream.")
	cptr := flag.String("c", "http://127.0.0.1:8080", "Upstream: lite. Should use unix upstream: unix:/dev/shm/hived.sock")
	fptr := flag.String("f", "http://127.0.0.1:8090", "Upstream: full/default.")
	hptr := flag.String("h", "", "Upstream: hivemind. Blank to disable.")
	pptr := flag.String("p", "", "Upstream: Push transaction. Blank to be equal to light upstream.")
	lptr := flag.String("l", "/dev/shm/hiveinterpreter.sock", "Listen sock location.")
	flag.Parse()
	debug = *dptr
	fullep = *fptr
	pushep = *pptr
	liteep = *cptr
	hiveep = *hptr
	workers = *wptr
	wQueue := *qptr
	listensock := *lptr

	// Create a separate worker queue for pushing regardless of if it is the same as the lite pool.
	var pushepDst string
	if pushep == "" {
		pushep = "pushPool"
		pushepDst = liteep
	} else {
		pushepDst = pushep
	}

	// Set up cache.
	respcache = cache.New(3*time.Second, 2*time.Minute)

	// Set up logging.
	f, err := os.OpenFile("hiveinterpreter.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Println(err)
	}
	defer f.Close()
	wrt := io.MultiWriter(os.Stdout, f)
	log.SetOutput(wrt)

	// Set up unix socket listener.
	os.Remove(listensock)
	unixListener, err := net.Listen("unix", listensock)
	if err != nil {
		log.Fatal("Listen (UNIX socket): ", err)
	}
	if err := os.Chmod(listensock, 0777); err != nil {
		log.Fatal(err)
	}
	defer unixListener.Close()

	// Set up upstreams.
	ep2pool = make(map[string]jobPool)
	ep2pool[fullep] = jobPool{initJobPool(workers, workers*wQueue), upstreamBuilder(fullep, "POST")}
	ep2pool[liteep] = jobPool{initJobPool(workers, workers*wQueue), upstreamBuilder(liteep, "POST")}
	ep2pool[hiveep] = jobPool{initJobPool(workers, workers*wQueue), upstreamBuilder(hiveep, "POST")}
	ep2pool[pushep] = jobPool{initJobPool(workers, workers*wQueue), upstreamBuilder(pushepDst, "POST")}

	// Handle incoming http requests.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { doHandleReg(w, r) })
	http.HandleFunc("/v1/", func(w http.ResponseWriter, r *http.Request) { doHandleREST(w, r) })

	if err := http.Serve(unixListener, nil); err != nil {
		log.Fatal(err)
	}
}
