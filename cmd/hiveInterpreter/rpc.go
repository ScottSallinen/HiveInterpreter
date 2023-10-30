package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	jsoniter "github.com/json-iterator/go"
)

// Condenser APIs considered to be 'light' and not requiring a full node.
var condenserliteAPIs = map[string]bool{
	"lookup_accounts":                   true,
	"get_config":                        true,
	"get_block":                         true,
	"get_block_header":                  true,
	"get_dynamic_global_properties":     true,
	"broadcast_block":                   true,
	"broadcast_transaction":             true,
	"broadcast_transaction_synchronous": true,
	"login":                             true,
	"find_rc_accounts":                  true,
	"get_active_witnesses":              true,
	"get_transaction_hex":               true,
	"get_version":                       true,
	"get_witness_by_account":            true,
	"get_witness_count":                 true,
	"get_witness_schedule":              true,
	"get_reward_fund":                   true,
	"get_potential_signatures":          true,
	"get_required_signatures":           true,
	"get_accounts":                      true,
	"get_vesting_delegations":           true,
	"get_witnesses_by_vote":             true,
	"get_current_median_history_price":  true,
	"get_withdraw_routes":               true,
	"get_feed_history":                  true,
	"get_account_reputations":           true,

	"get_key_references": true,
	"get_owner_history":  true,

	"get_market_history":          true,
	"get_market_history_buckets":  true,
	"get_order_book":              true,
	"get_recent_trades":           true,
	"get_ticker":                  true,
	"get_trade_history":           true,
	"get_volume":                  true,
	"get_hardfork_version":        true,
	"verify_authority":            true,
	"get_witnesses":               true,
	"get_next_scheduled_hardfork": true,
}

// Appbase APIs considered to be 'light' and not requiring a full node.
var abliteAPIs = map[string]bool{
	"rc_api":                true,
	"block_api":             true,
	"chain_api":             true,
	"database_api":          true,
	"network_broadcast_api": true,
	"reputation_api":        true,

	"account_by_key_api":     true,
	"market_history_api":     true,
	"transaction_status_api": true,
	"wallet_bridge_api":      true,
}

// Appbase APIs that are handled by hivemind.
var abhiveAPIs = map[string]bool{
	"tags_api":   true,
	"follow_api": true,
}

// APIs that are handled by hivemind.
var hiveAPIs = map[string]bool{
	"get_followers":    true,
	"get_following":    true,
	"get_follow_count": true,

	"get_content":         true,
	"get_content_replies": true,
	"get_active_votes":    true,

	"get_state":      true,
	"get_discussion": true,

	"get_trending_tags": true,

	"get_discussions_by_trending": true,
	"get_discussions_by_hot":      true,
	"get_discussions_by_promoted": true,
	"get_discussions_by_created":  true,

	"get_discussions_by_blog":     true,
	"get_discussions_by_feed":     true,
	"get_discussions_by_comments": true,
	"get_replies_by_last_update":  true,

	"get_blog":                              true,
	"get_blog_entries":                      true,
	"get_discussions_by_author_before_date": true,
	"get_post_discussions_by_payout":        true,
	"get_comment_discussions_by_payout":     true,
	"get_account_votes":                     true,

	"get_reblogged_by": true,
}

// Override the default cache time for certain APIs.
var cacheTime = map[string]int{
	"get_ranked_posts":     15,
	"get_discussion":       9,
	"get_account_posts":    15,
	"get_profile":          30,
	"get_state":            9,
	"get_content":          6,
	"get_content_replies":  6,
	"get_active_votes":     6,
	"unread_notifications": 60,
}

// Handle a REST request. This is interpreted to the appropriate json RPC call.
func doHandleREST(w http.ResponseWriter, r *http.Request) {
	mark := time.Now()

	inpath := r.URL.EscapedPath()
	api, api_method := path.Split(inpath)
	api_v, api_call := path.Split(path.Clean(api))
	api_v = path.Clean(api_v)

	/// Default catch-all.
	target_url := fullep

	if api_v != "/v1" {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if api_call == "" || api_method == "" {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	fparams := Flatten(r.URL.Query())

	if api_call == "hive" || api_call == "bridge" {
		target_url = hiveep
	}
	if hiveAPIs[api_method] {
		target_url = hiveep
		api_call = "condenser_api"
	}
	if abliteAPIs[api_call] {
		target_url = liteep
	}
	reqmessage := map[string]interface{}{"id": "0", "jsonrpc": "2.0", "method": api_call + "." + api_method, "params": fparams}

	if api_method == "get_block_by_time" {
		params := r.URL.Query()
		getBlockByTime(target_url, params, w, mark)
		return
	}

	if api_method == "get_total_supply" {
		getTotalSupply(target_url, "virtual_supply", w)
		return
	}

	if api_method == "get_circulating_supply" {
		getTotalSupply(target_url, "current_supply", w)
		return
	}

	if api_method == "get_original_body" {
		fparams := Flatten(r.URL.Query())
		getOriginalBody(target_url, fparams, w, mark)
		return
	}

	requestJson, err := jsonit.Marshal(reqmessage)
	if err != nil {
		log.Println("Couldn't marshal request")
		log.Println(err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	// Have both request and target here.
	var resp map[string]interface{}
	var respreal map[string]interface{}
	var respJson []byte
	key := string(requestJson)
	var status int
	var gcached bool
	if x, found := respcache.Get(key); found {
		respJson = x.([]byte)
		gcached = true
	} else {
		status, respJson = requestToResponseBytes(ep2pool[target_url], requestJson)
		if status != http.StatusOK {
			http.Error(w, http.StatusText(status), status)
			return
		}
		err := jsonit.Unmarshal(respJson, &resp)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		respreal = resp
		if resp["result"] == nil {
			if resp["error"] != nil {
				respreal, _ = (resp["error"]).(map[string]interface{})
			}
		} else {
			var ok bool
			respreal, ok = (resp["result"]).(map[string]interface{})
			if !ok {
				respreal = resp
				delete(respreal, "id")
				delete(respreal, "jsonrpc")
			}
		}
		respJson, _ = json.MarshalIndent(respreal, "", "  ")
		respcache.SetDefault(key, respJson)
		gcached = false
	}

	// Finalize reply, convert back from json, and write.
	w.Header().Set("Content-Type", "application/json")
	w.Write(respJson)

	if debug {
		elapsed := time.Since(mark)
		log.Println(elapsed, gcached, target_url, "-d '"+string(requestJson)+"'")
	}
}

// Handles an incoming http request, in standard hive json RPC format. Is normalized then sent to the appropriate endpoint.
func doHandleReg(w http.ResponseWriter, r *http.Request) {
	mark := time.Now()

	// Unpack request into json.
	var f interface{}
	err := jsonit.NewDecoder(r.Body).Decode(&f)
	switch {
	case err == io.EOF:
		respmessage := map[string]string{"status": "OK", "jussi_num": "-1", "info": "For information on how to use this api, visit https://developers.hive.io/apidefinitions/ "}
		w.Header().Set("Content-Type", "application/json")
		jsonit.NewEncoder(w).Encode(respmessage)
		return
	case err != nil:
		log.Println("Couldn't unpack json.")
		log.Println(err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	defer func() {
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
	}()
	var reqmessage map[string]interface{}
	var ok bool
	arrayreq := false

	reqmessage, ok = f.(map[string]interface{})
	if !ok {
		newf, ok := f.([]interface{})
		if !ok {
			log.Println("Couldn't type outer json")
			log.Println(f)
			log.Println(err)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		reqmessage, ok = newf[0].(map[string]interface{})
		if !ok {
			log.Println("Couldn't type inner json")
			log.Println(newf)
			log.Println(err)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		if len(f.([]interface{})) > 1 {
			http.Error(w, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
			return
		}
		arrayreq = true
	}

	// add jsonrpc if not in message
	if _, ok := reqmessage["jsonrpc"]; !ok {
		reqmessage["jsonrpc"] = "2.0"
	}
	// add id if not in message
	if _, ok := reqmessage["id"]; !ok {
		reqmessage["id"] = "0"
	}

	old_id := reqmessage["id"]
	reqmessage["id"] = "0"

	standaloneMethod := ""

	method, ok := reqmessage["method"].(string)
	if !ok {
		log.Println("Couldn't type method")
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	// Convert and sanitize request json -- map to target upstream based on request.
	target_url := fullep
	if reqmessage["method"] != "call" && !strings.Contains(method, ".") {
		reqmessage["params"] = append([]interface{}{reqmessage["method"]}, reqmessage["params"])
		reqmessage["params"] = append([]interface{}{"condenser_api"}, reqmessage["params"].([]interface{})...)
		reqmessage["method"] = "call"
	}

	if reqmessage["method"] == "call" {
		params, ok := reqmessage["params"].([]interface{})
		if !ok {
			//log.Println("Couldn't type params from: ", reqmessage)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		if params[0] == 0 {
			params[0] = "database_api"
		} else if params[0] == 1 {
			params[0] = "login_api"
		}
		cond_meth, ok := params[1].(string)
		if !ok {
			log.Println("Couldn't type condenser params from: ", reqmessage)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		standaloneMethod = cond_meth

		if len(params) <= 2 {
			if condenserliteAPIs[cond_meth] {
				target_url = liteep
			}
		} else {
			switch callparams := params[2].(type) {
			case []interface{}:
				params[0] = "condenser_api"
				if condenserliteAPIs[cond_meth] {
					target_url = liteep
				}
				if pushep != "" && (cond_meth == "broadcast_transaction_synchronous" || cond_meth == "broadcast_transaction") {
					target_url = pushep
				}
				if hiveep != "" && hiveAPIs[cond_meth] {
					target_url = hiveep
					if cond_meth == "get_state" {
						if len(callparams) >= 1 {
							pp, ok := callparams[0].(string)
							if ok {
								lmatch, _ := regexp.MatchString(`^\/?(~?witnesses|proposals)$`, pp)
								if lmatch {
									target_url = liteep
								}
								match, _ := regexp.MatchString(`/@[^/]+/transfers`, pp)
								if match {
									target_url = fullep
								}
							}
						}
					}
				}
				if cond_meth == "get_transaction" {
					//thetrx = callparams[0]
					reqmessage["method"] = "condenser_api.get_transaction"
					reqmessage["params"] = callparams
				}
				if cond_meth == "get_account_history" {
					if len(callparams) >= 2 {
						mnum64, mok64 := MaybeGetInt64(callparams[2])
						if mok64 && mnum64 > 10000 {
							http.Error(w, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
							return
						}
					}
				}
			case map[string]interface{}: //oops, its actually appbase...
				cond_api, ok := params[0].(string)
				if !ok {
					//log.Println("Couldn't type appbase params from: ", reqmessage)
					http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
					return
				}
				reqmessage["method"] = cond_api + "." + cond_meth
				reqmessage["params"] = callparams
			default:
				log.Println("Couldn't type call params")
				http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
				return
			}
		}
	}

	// Limit account history to 10k entries.
	if reqmessage["method"] == "condenser_api.get_account_history" {
		params, ok := reqmessage["params"].([]interface{})
		if !ok {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		}
		if len(params) >= 2 {
			mnum64, mok64 := MaybeGetInt64(params[2])
			if mok64 && mnum64 > 10000 {
				http.Error(w, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
				return
			}
		}
	}

	// Limit block range to 1.
	if reqmessage["method"] == "block_api.get_block_range" {
		params, ok := reqmessage["params"].(map[string]interface{})
		if !ok {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		}
		bcnt, ok := params["count"]
		if !ok {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		}
		ibcnt, ok := MaybeGetInt64(bcnt)
		if !ok {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		}
		if ibcnt != 1 {
			http.Error(w, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
			return
		}
	}

	if pushep != "" && (reqmessage["method"] == "condenser_api.broadcast_transaction_synchronous" || reqmessage["method"] == "condenser_api.broadcast_transaction") {
		target_url = pushep
	}

	slp := strings.Split(reqmessage["method"].(string), ".")
	if len(slp) > 1 {
		standaloneMethod = slp[1]
	}

	if abliteAPIs[slp[0]] {
		target_url = liteep
	}
	if pushep != "" && slp[0] == "network_broadcast_api" {
		target_url = pushep
	}

	if slp[0] == "condenser_api" && len(slp) > 1 {
		if condenserliteAPIs[slp[1]] {
			target_url = liteep
		}
	}

	if hiveep != "" {
		if slp[0] == "hive" || slp[0] == "bridge" {
			target_url = hiveep
		}
		if len(slp) > 1 {
			if abhiveAPIs[slp[0]] {
				if slp[1] != "get_active_votes" {
					target_url = hiveep
				} else {
					reqmessage["method"] = "condenser_api." + slp[1]
				}
			} else if hiveAPIs[slp[1]] {
				target_url = hiveep
			}
			if slp[1] == "get_state" {
				params, ok := reqmessage["params"].([]interface{})
				if ok {
					if len(params) >= 1 {
						lmatch, _ := regexp.MatchString(`^\/?(~?witnesses|proposals)$`, params[0].(string))
						if lmatch {
							target_url = liteep
						}
						match, _ := regexp.MatchString(`/@[^/]+/transfers`, params[0].(string))
						if match {
							target_url = fullep
						}
					}
				}
			}
		}
	}

	// Pack the normalized request as json.
	requestJson, err := jsonit.Marshal(reqmessage)
	if err != nil {
		log.Println("Couldn't re-marshal request")
		log.Println(err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	// Have both request and target here.
	var resp map[string]interface{}
	var respJson []byte
	key := string(requestJson)
	var status int
	var gcached bool
	if x, found := respcache.Get(key); found {
		respJson = x.([]byte)
		gcached = true
	} else {
		status, respJson = requestToResponseBytes(ep2pool[target_url], requestJson)
		if status != http.StatusOK {
			http.Error(w, http.StatusText(status), status)
			return
		}
		if mCacheTime, fnd := cacheTime[standaloneMethod]; fnd {
			respcache.Set(key, respJson, time.Duration(mCacheTime)*time.Second)
		} else {
			respcache.SetDefault(key, respJson)
		}
		gcached = false
	}

	// Finalize reply, convert back from json, and write.
	if old_id != "0" || arrayreq {
		jsonit.Unmarshal(respJson, &resp)
		resp["id"] = old_id

		// Check for database lock error.
		if val, in := resp["error"]; in {
			if valmap, ok := val.(map[string]interface{}); ok {
				if codei, ok := jsoniter.CastJsonNumber(valmap["code"]); ok {
					if i, err := strconv.Atoi(codei); err != nil && i == -32003 { // database lock
						log.Println("Req errored: ", val)
						log.Println("From: ", string(requestJson))
					}
				}
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")

	if arrayreq {
		var rmm []interface{}
		rmm = append(rmm, resp)
		jsonit.NewEncoder(w).Encode(rmm)
	} else {
		if old_id != "0" {
			jsonit.NewEncoder(w).Encode(resp)
		} else {
			w.Write(respJson)
		}
	}

	elapsed := time.Since(mark)

	if int(elapsed/time.Second) >= 5 {
		log.Println("LONG:", elapsed, gcached, target_url, "-d '"+string(requestJson)+"'")
	}

	if debug {
		log.Println(elapsed, gcached, target_url, "-d '"+string(requestJson)+"'")
	}
}
