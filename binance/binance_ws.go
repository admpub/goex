package binance

import (
	"errors"
	"fmt"
	"github.com/json-iterator/go"
	. "github.com/nntaoli-project/GoEx"
	"net/http"
	"strings"
	"sync"
	"time"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type BinanceWs struct {
	*WsBuilder
	sync.Once
	wsConn *WsConn

	baseURL         string
	combinedBaseURL string
	tickerCallback  func(*Ticker)
	depthCallback   func(*Depth)
	tradeCallback   func(*Trade)
	klineCallback   func(*Kline, int)

	tradeSymbols []TradeSymbol
}

var _INERNAL_KLINE_PERIOD_REVERTER = map[string]int{
	"1m":  KLINE_PERIOD_1MIN,
	"3m":  KLINE_PERIOD_3MIN,
	"5m":  KLINE_PERIOD_5MIN,
	"15m": KLINE_PERIOD_15MIN,
	"30m": KLINE_PERIOD_30MIN,
	"1h":  KLINE_PERIOD_60MIN,
	"2h":  KLINE_PERIOD_2H,
	"4h":  KLINE_PERIOD_4H,
	"6h":  KLINE_PERIOD_6H,
	"8h":  KLINE_PERIOD_8H,
	"12h": KLINE_PERIOD_12H,
	"1d":  KLINE_PERIOD_1DAY,
	"3d":  KLINE_PERIOD_3DAY,
	"1w":  KLINE_PERIOD_1WEEK,
	"1M":  KLINE_PERIOD_1MONTH,
}

func NewBinanceWs(client *http.Client) *BinanceWs {
	bn := New(client, "", "")

	bnWs := &BinanceWs{}
	bnWs.baseURL = "wss://stream.binance.com:9443/ws"
	bnWs.combinedBaseURL = "wss://stream.binance.com:9443/stream?streams="
	bnWs.WsBuilder = NewWsBuilder().
		WsUrl(bnWs.baseURL).
		ReconnectIntervalTime(24 * time.Hour).
		UnCompressFunc(FlateUnCompress).
		ProtoHandleFunc(bnWs.handle)
	var err error
	bnWs.tradeSymbols, err = bn.getTradeSymbols()
	if len(bnWs.tradeSymbols) == 0 || err != nil {
		panic("trade symbol is empty, pls check connection...")
	}

	return bnWs
}

func (bnWs *BinanceWs) SetBaseUrl(baseURL string) {
	bnWs.baseURL = baseURL
}

func (bnWs *BinanceWs) SetCombinedBaseURL(combinedBaseURL string) {
	bnWs.combinedBaseURL = combinedBaseURL
}

func (bnWs *BinanceWs) SetCallbacks(
	tickerCallback func(*Ticker),
	depthCallback func(*Depth),
	tradeCallback func(*Trade),
	klineCallback func(*Kline, int),
) {
	bnWs.tickerCallback = tickerCallback
	bnWs.depthCallback = depthCallback
	bnWs.tradeCallback = tradeCallback
	bnWs.klineCallback = klineCallback
}

func (bnWs *BinanceWs) subscribe(sub string) error {
	bnWs.connectWs()
	return bnWs.wsConn.Subscribe(sub)
}

func (bnWs *BinanceWs) SubscribeDepth(pair CurrencyPair, size int) error {
	if bnWs.depthCallback == nil {
		return errors.New("please set depth callback func")
	}
	if size != 5 && size != 10 && size != 20 {
		return errors.New("please set depth size as 5 / 10 / 20")
	}
	endpoint := fmt.Sprintf("%s/%s@depth<%d>", bnWs.baseURL, strings.ToLower(pair.ToSymbol("")), size)

	return bnWs.subscribe(endpoint)
}

func (bnWs *BinanceWs) SubscribeTicker(pair CurrencyPair) error {
	if bnWs.tickerCallback == nil {
		return errors.New("please set ticker callback func")
	}
	endpoint := fmt.Sprintf("%s/%s@ticker", bnWs.baseURL, strings.ToLower(pair.ToSymbol("")))
	return bnWs.subscribe(endpoint)
}

func (bnWs *BinanceWs) SubscribeTrade(pair CurrencyPair) error {
	if bnWs.tradeCallback == nil {
		return errors.New("please set trade callback func")
	}
	endpoint := fmt.Sprintf("%s/%s@trade", bnWs.baseURL, strings.ToLower(pair.ToSymbol("")))
	return bnWs.subscribe(endpoint)
}

func (bnWs *BinanceWs) SubscribeKline(pair CurrencyPair, period int) error {
	if bnWs.klineCallback == nil {
		return errors.New("place set kline callback func")
	}
	periodS, isOk := _INERNAL_KLINE_PERIOD_CONVERTER[period]
	if isOk != true {
		periodS = "M1"
	}
	endpoint := fmt.Sprintf("%s/%s@kline_%s", bnWs.baseURL, strings.ToLower(pair.ToSymbol("")), periodS)
	return bnWs.subscribe(endpoint)
}

func (bnWs *BinanceWs) connectWs() {
	bnWs.Do(func() {
		bnWs.wsConn = bnWs.WsBuilder.Build()
		bnWs.wsConn.ReceiveMessage()
	})
}

func (bnWs *BinanceWs) parseTickerData(tickmap map[string]interface{}) *Ticker {
	t := new(Ticker)
	t.Date = ToUint64(tickmap["E"])
	t.Last = ToFloat64(tickmap["c"])
	t.Vol = ToFloat64(tickmap["v"])
	t.Low = ToFloat64(tickmap["l"])
	t.High = ToFloat64(tickmap["h"])
	t.Buy = ToFloat64(tickmap["b"])
	t.Sell = ToFloat64(tickmap["a"])

	return t
}

func (bnWs *BinanceWs) parseDepthData(bids, asks []interface{}) *Depth {
	depth := new(Depth)
	//n := 0
	//for i := 0; i < len(bids); {
	//	depth.BidList = append(depth.BidList, DepthRecord{ToFloat64(bids[i]), ToFloat64(bids[i+1])})
	//	i += 2
	//	n++
	//}
	//
	//n = 0
	//for i := 0; i < len(asks); {
	//	depth.AskList = append(depth.AskList, DepthRecord{ToFloat64(asks[i]), ToFloat64(asks[i+1])})
	//	i += 2
	//	n++
	//}

	return depth
}

func (bnWs *BinanceWs) parseKlineData(k map[string]interface{}) *Kline {
	kline := &Kline{
		Timestamp: int64(ToInt(k["t"])),
		Open:      ToFloat64(k["o"]),
		Close:     ToFloat64(k["c"]),
		High:      ToFloat64(k["h"]),
		Low:       ToFloat64(k["l"]),
		Vol:       ToFloat64(k["v"]),
	}
	return kline
}

func (bnWs *BinanceWs) handle(msg []byte) error {
	//fmt.Println("ws msg:", string(msg))
	datamap := make(map[string]interface{})
	err := json.Unmarshal(msg, &datamap)
	if err != nil {
		fmt.Println("json unmarshal error for ", string(msg))
		return err
	}

	msgType, isOk := datamap["e"].(string)
	if !isOk {
		return errors.New("no message type")
	}

	switch msgType {
	case "24hrTicker":
		tick := bnWs.parseTickerData(datamap)
		pair, err := bnWs.getPairFromType(datamap["s"].(string))
		if err != nil {
			panic(err)
		}
		tick.Pair = pair
		bnWs.tickerCallback(tick)
		return nil
	//case "depth":
	//	dep := bnWs.parseDepthData(datamap["bids"].([]interface{}), datamap["asks"].([]interface{}))
	//	stime := int64(ToInt(datamap["ts"]))
	//	dep.UTime = time.Unix(stime/1000, 0)
	//	pair, err := bnWs.getPairFromType(resp[2])
	//	if err != nil {
	//		panic(err)
	//	}
	//	dep.Pair = pair
	//
	//	bnWs.depthCallback(dep)
	//	return nil
	case "kline":
		k := datamap["k"].(map[string]interface{})
		period := _INERNAL_KLINE_PERIOD_REVERTER[k["i"].(string)]
		kline := bnWs.parseKlineData(k)
		pair, err := bnWs.getPairFromType(datamap["s"].(string))
		if err != nil {
			panic(err)
		}
		kline.Pair = pair
		bnWs.klineCallback(kline, period)
		return nil
	case "trade":
		side := BUY
		if datamap["m"].(bool) == false {
			side = SELL
		}
		trade := &Trade{
			Tid:    int64(ToUint64(datamap["t"])),
			Type:   TradeSide(side),
			Amount: ToFloat64(datamap["q"]),
			Price:  ToFloat64(datamap["p"]),
			Date:   int64(ToUint64(datamap["E"])),
		}
		pair, err := bnWs.getPairFromType(datamap["s"].(string))
		if err != nil {
			panic(err)
		}
		trade.Pair = pair
		bnWs.tradeCallback(trade)
		return nil
	default:
		return errors.New("unknown message " + msgType)

	}
	return nil
}

func (bnWs *BinanceWs) getPairFromType(pair string) (CurrencyPair, error) {
	for _, v := range bnWs.tradeSymbols {
		if v.Symbol == pair {
			return NewCurrencyPair2(v.BaseAsset + "_" + v.QuoteAsset), nil
		}
	}
	return NewCurrencyPair2("" + "_" + ""), errors.New("pair not support :" + pair)
}
