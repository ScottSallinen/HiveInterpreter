package main

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/sergi/go-diff/diffmatchpatch"
)

// Provides a simple interface to the supply from get_dynamic_global_properties.
func getTotalSupply(targetUrl string, supplyType string, w http.ResponseWriter) {
	params := map[string]interface{}{}
	reqmessage := map[string]interface{}{"id": "0", "jsonrpc": "2.0", "method": "database_api" + "." + "get_dynamic_global_properties", "params": params}
	status, resp := requestToResponse(ep2pool[targetUrl], reqmessage)
	if status != http.StatusOK {
		http.Error(w, http.StatusText(status), status)
		return
	}
	sup := (((resp["result"]).(map[string]interface{}))[supplyType]).(map[string]interface{})
	realAmount := sup["amount"].(string)
	toWrite := realAmount[:len(realAmount)-3]
	toWrite = toWrite + "."
	toWrite = toWrite + realAmount[len(realAmount)-3:]
	w.Write([]byte(toWrite))
}

// Retrives the block that occured at the given timestamp. Needs to do some searching for it.
func getBlockByTime(targetUrl string, inputParams url.Values, w http.ResponseWriter, mark time.Time) {
	if inputParams["timestamp"] == nil || len(inputParams["timestamp"]) != 1 {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	status, btarget := getBlockByTimeHelper(ep2pool[targetUrl], inputParams["timestamp"][0])
	if status != http.StatusOK {
		http.Error(w, http.StatusText(status), status)
		return
	}

	//log.Println("btarget", btarget, "\n")
	params := map[string]interface{}{"block_num": btarget}
	reqmessage := map[string]interface{}{"id": "0", "jsonrpc": "2.0", "method": "block_api" + "." + "get_block", "params": params}
	status, rresp := requestToResponse(ep2pool[targetUrl], reqmessage)
	if status != http.StatusOK {
		http.Error(w, http.StatusText(status), status)
		return
	}
	(((rresp["result"]).(map[string]interface{}))["block"]).(map[string]interface{})["block"] = btarget
	// Finalize reply, convert back from json, and write.
	w.Header().Set("Content-Type", "application/json")
	respj, _ := json.MarshalIndent(rresp, "", "  ")
	w.Write(respj)
	if debug {
		elapsed := time.Since(mark)
		delete(reqmessage, "id")
		delete(reqmessage, "jsonrpc")
		jm, _ := jsonit.Marshal(reqmessage)
		log.Println(elapsed, string(jm))
	}
}

// Helper function for getBlockByTime. Does the actual searching.
func getBlockByTimeHelper(jobp jobPool, reqtime string) (int, int) {
	tsInit := "2016-03-24T16:05:00"
	layout := "2006-01-02T15:04:05"
	t1, _ := time.Parse(layout, tsInit)
	t2, err := time.Parse(layout, reqtime)
	if err != nil {
		return http.StatusBadRequest, 0
	}
	diff := t2.Sub(t1)
	bguess := int(diff.Seconds() / 3)
	//log.Println("bguess: ", bguess)
	params := map[string]interface{}{}
	btarget := 0
	if diff < 0 {
		btarget = 1
	}
	if bguess == 0 {
		bguess = 1
	}

	reqmessage := map[string]interface{}{"id": "0", "jsonrpc": "2.0", "method": "database_api" + "." + "get_dynamic_global_properties", "params": params}
	status, resp := requestToResponse(jobp, reqmessage)
	if status != http.StatusOK {
		return status, 0
	}
	head, ok := (((resp["result"]).(map[string]interface{}))["time"]).(string)
	headt, _ := time.Parse(layout, head)
	if !ok {
		return http.StatusBadRequest, 0
	}

	if t2.Sub(headt) > 0 {
		jbt, _ := jsoniter.CastJsonNumber((((resp["result"]).(map[string]interface{}))["head_block_number"]))
		btarget, _ = strconv.Atoi(jbt)
	}

	bdeltaprev := 0
	bconst := int(math.Pow(2, 23)) // for refinement
	for btarget == 0 {
		params = map[string]interface{}{"block_num": bguess}
		reqmessage = map[string]interface{}{"id": "0", "jsonrpc": "2.0", "method": "block_api" + "." + "get_block_header", "params": params}

		status, resp := requestToResponse(jobp, reqmessage)
		if status != http.StatusOK {
			return status, 0
		}

		if (resp["result"]).(map[string]interface{})["header"] == nil { // too far
			bguess = bguess - bconst
			bconst = int(bconst / 2)
			if bconst == 0 {
				btarget = 1
				break
			}
			continue
		}
		newts := (((resp["result"]).(map[string]interface{}))["header"]).(map[string]interface{})["timestamp"]
		newtss, ok := newts.(string)
		if !ok {
			return http.StatusBadRequest, 0
		}
		t3, _ := time.Parse(layout, newtss)
		tdelta := t3.Sub(t2) // how far ahead we are
		bdelta := int(tdelta.Seconds() / 3)
		if bdelta == 0 {
			btarget = bguess
			break
		}
		bguess = bguess - bdelta
		if bdeltaprev == -1*bdelta {
			btarget = bguess
			break
		}
		bdeltaprev = bdelta
	}
	return http.StatusOK, btarget
}

// Returns the original body of a post, even if it has been edited. Uses the block by time helper function.
func getOriginalBody(targetUrl string, fparams map[string]interface{}, w http.ResponseWriter, mark time.Time) {
	reqmessage := map[string]interface{}{"id": "0", "jsonrpc": "2.0", "method": "condenser_api" + "." + "get_content", "params": []interface{}{fparams["author"], fparams["permlink"]}}
	status, resp := requestToResponse(ep2pool[targetUrl], reqmessage)
	if status != http.StatusOK {
		http.Error(w, http.StatusText(status), status)
		return
	}
	created := (((resp["result"]).(map[string]interface{}))["created"]).(string)
	last_update, ok := (((resp["result"]).(map[string]interface{}))["last_update"]).(string)

	if !ok {
		// Finalize reply, convert back from json, and write.
		w.Header().Set("Content-Type", "application/json")
		respj, _ := json.MarshalIndent(resp, "", "  ")
		w.Write(respj)
		if debug {
			elapsed := time.Since(mark)
			delete(reqmessage, "id")
			delete(reqmessage, "jsonrpc")
			jm, _ := jsonit.Marshal(reqmessage)
			log.Println(elapsed, string(jm))
		}
		return
	}

	oldbdy, _ := (((resp["result"]).(map[string]interface{}))["body"]).(string)
	if created == last_update {
		respmessage := map[string]interface{}{"body": oldbdy, "edited": false}
		// Finalize reply, convert back from json, and write.
		w.Header().Set("Content-Type", "application/json")
		respj, _ := json.MarshalIndent(respmessage, "", "  ")
		w.Write(respj)
		if debug {
			elapsed := time.Since(mark)
			delete(reqmessage, "id")
			delete(reqmessage, "jsonrpc")
			jm, _ := jsonit.Marshal(reqmessage)
			log.Println(elapsed, string(jm))
		}
		return
	}

	status, btarget := getBlockByTimeHelper(ep2pool[targetUrl], created)
	if status != http.StatusOK {
		http.Error(w, http.StatusText(status), status)
		return
	}

	params := map[string]interface{}{"block_num": btarget + 1}
	reqmessage = map[string]interface{}{"id": "0", "jsonrpc": "2.0", "method": "block_api" + "." + "get_block", "params": params}
	status, resp = requestToResponse(ep2pool[targetUrl], reqmessage)
	if status != http.StatusOK {
		http.Error(w, http.StatusText(status), status)
		return
	}

	trxs := ((resp["result"].(map[string]interface{})["block"]).(map[string]interface{})["transactions"]).([]interface{})
	for _, trx := range trxs {
		rtrx, _ := (trx).(map[string]interface{})
		ops := (rtrx["operations"]).([]interface{})
		for _, op := range ops {
			rop, _ := (op).(map[string]interface{})
			if rop["type"] == "comment_operation" {
				ropv := (rop["value"]).(map[string]interface{})
				if ropv["author"] == fparams["author"] && ropv["permlink"] == fparams["permlink"] {
					bdy, _ := ropv["body"].(string)
					dmp := diffmatchpatch.New()
					diffs := dmp.DiffMain(bdy, oldbdy, false)
					respmessage := map[string]interface{}{"body": bdy, "edited": true, "diff_to_latest": dmp.DiffToDelta(diffs)}
					// Finalize reply, convert back from json, and write.
					w.Header().Set("Content-Type", "application/json")
					respj, _ := json.MarshalIndent(respmessage, "", "  ")
					w.Write(respj)
					if debug {
						elapsed := time.Since(mark)
						delete(reqmessage, "id")
						delete(reqmessage, "jsonrpc")
						jm, _ := jsonit.Marshal(reqmessage)
						log.Println(elapsed, string(jm))
					}
					return
				}
			}
		}
	}

	respmessage := map[string]interface{}{"error": "not found"}
	// Finalize reply, convert back from json, and write.
	w.Header().Set("Content-Type", "application/json")
	respj, _ := json.MarshalIndent(respmessage, "", "  ")
	w.Write(respj)
}
