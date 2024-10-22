package ichimoku

import (
	"context"
	"fmt"

	"github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	"github.com/sirupsen/logrus"
)

const ID = "ichimoku_cloud"

var log = logrus.WithField("strategy", ID)

type Strategy struct {
	//	*common.Strategy
	*bbgo.Notifiability
	session       *bbgo.ExchangeSession
	orderExecutor bbgo.OrderExecutor
	Environment   *bbgo.Environment
	Market        types.Market
	Symbol        string `json:"symbol"`
	Current       types.KLine
	Interval      types.Interval `json:"interval"`
	AutoTrade     bool           `json:"autoTrade"`
	PreInterval   types.Interval `json:"preInterval"`
	//UpperPrice fixedpoint.Value `json:"upperPrice" yaml:"upperPrice"`

	ConversionPeriod  int `json:"conversion_period" yaml:"conversion_period"`
	BasePeriod        int `json:"base_period" yaml:"base_period"`
	LaggingSpanPeriod int `json:"lagging_span_period" yaml:"lagging_span_period"`
	Displacement      int `json:"displacement" yaml:"displacement"`
	klines            []types.KLine
}

func (s *Strategy) Subscribe(session *bbgo.ExchangeSession) {
	session.Subscribe(types.KLineChannel, s.Symbol, types.SubscribeOptions{Interval: s.Interval})
	session.Subscribe(types.KLineChannel, s.Symbol, types.SubscribeOptions{Interval: s.PreInterval})
}

func (s *Strategy) ID() string {
	return ID
}

func (s *Strategy) InstanceID() string {
	return fmt.Sprintf("%s:%s:%s", ID, s.Symbol, s.Interval)
}

func (s *Strategy) calculateIchimokuCloud() (tenkanSen, kijunSen, senkouSpanA, senkouSpanB, chikouSpan []float64) {
	candles := s.klines
	candles = append(candles, s.Current)
	length := len(candles)
	tenkanSen = make([]float64, length)
	kijunSen = make([]float64, length)
	senkouSpanA = make([]float64, length)
	senkouSpanB = make([]float64, length)
	chikouSpan = make([]float64, length)

	for i := range candles {

		if i >= s.ConversionPeriod-1 {
			tenkanSen[i] = (highestHigh(candles[i-s.ConversionPeriod+1:i+1]) + lowestLow(candles[i-s.ConversionPeriod+1:i+1])) / 2
		}
		if i >= s.BasePeriod-1 {
			kijunSen[i] = (highestHigh(candles[i-s.BasePeriod+1:i+1]) + lowestLow(candles[i-s.BasePeriod+1:i+1])) / 2
		}
		if i >= s.BasePeriod-1 {
			senkouSpanA[i] = (tenkanSen[i] + kijunSen[i]) / 2
		}
		if i >= s.LaggingSpanPeriod-1 {
			senkouSpanB[i] = (highestHigh(candles[i-s.LaggingSpanPeriod+1:i+1]) + lowestLow(candles[i-s.LaggingSpanPeriod+1:i+1])) / 2
		}
		if i >= s.Displacement {
			chikouSpan[i-s.Displacement] = candles[i].Close.Float64()
		}
	}

	return tenkanSen, kijunSen, senkouSpanA, senkouSpanB, chikouSpan
}

func (s *Strategy) Run(ctx context.Context, orderExecutor bbgo.OrderExecutor, session *bbgo.ExchangeSession) error {
	//s.Strategy.Initialize(ctx, s.Environment, session, s.Market, ID, s.InstanceID())
	log.Info("start")
	s.session = session
	s.orderExecutor = orderExecutor

	kLineStore, _ := session.MarketDataStore(s.Symbol)

	if klines, ok := kLineStore.KLinesOfInterval(s.Interval); ok {
		s.klines = (*klines)[0:]
	}

	last := s.klines[len(s.klines)-1]
	s.Current = types.KLine{
		Open:  last.Close,
		High:  last.Close,
		Low:   last.Close,
		Close: last.Close,
	}

	minTradeAmount, _ := fixedpoint.NewFromString("10")

	session.MarketDataStream.OnKLineClosed(func(k types.KLine) {
		if k.Interval.String() != s.Interval.String() && k.Interval.String() != s.PreInterval.String() {
			return
		}
		if k.Interval == s.Interval {
			s.klines = append(s.klines, k)
			s.Current = types.KLine{
				Open:      k.Close,
				High:      k.Close,
				Low:       k.Close,
				Close:     k.Close,
				StartTime: k.StartTime,
			}
			return
		} else {
			if s.Current.High < k.Close {
				s.Current.High = k.Close
			}

			if s.Current.Low > k.Close {
				s.Current.Low = k.Close
			}

			s.Current.Close = k.Close
		}
		tenkanSen, kijunSen, senkouSpanA, senkouSpanB, _ := s.calculateIchimokuCloud()
		price := k.Close.Float64()
		cloudTop := max(senkouSpanA[len(senkouSpanA)-s.Displacement], senkouSpanB[len(senkouSpanB)-s.Displacement])
		cloudBottom := min(senkouSpanA[len(senkouSpanA)-s.Displacement], senkouSpanB[len(senkouSpanB)-s.Displacement])
		//log.Debugf("%v ichimoku cloud price %v cloudtop %v cloudBottom%v, conversion line %v baseline  %v\n", k.StartTime, price, cloudTop, cloudBottom, tenkanSen[len(tenkanSen)-1], kijunSen[len(kijunSen)-1])

		/*
		  tenkanSen = convertion
		  kijunSen = base
		  cloudtop = leading SpanA
		  cloudBottom =  leading SpanB

		*/
		//if tenkanSen[len(tenkanSen)-1] > kijunSen[len(kijunSen)-1] && price > cloudTop && chikouSpan[len(chikouSpan)-s.Displacement] > price {
		if tenkanSen[len(tenkanSen)-1] > kijunSen[len(kijunSen)-1] && price > cloudTop {
			//if price > cloudTop {
			//log.Infof("buy signal")
			balance, _ := s.session.Account.Balance("USDT")
			if balance.Available.Compare(0) > 0 && balance.Available.Compare(minTradeAmount) > 0 && s.AutoTrade {
				log.Infof("%v ichimoku cloud price %v cloudtop %v cloudBottom%v, conversion line %v baseline  %v\n", k.StartTime, price, cloudTop, cloudBottom, tenkanSen[len(tenkanSen)-1], kijunSen[len(kijunSen)-1])
				qty := balance.Available.Div(k.Close)
				s.placeOrder(ctx, types.SideTypeBuy, qty, k.Symbol)
				//log.Infof("buy BTC with %v USDT in %v at %v", balance.Available, k.Close, k.EndTime)
				s.Notify("buy BTC with %v USDT in %v at %v", balance.Available, k.Close, k.StartTime.Time())
			}
			s.Notify("Buy %v signal at %v", k.Symbol, k.Close)
			//} else if tenkanSen[len(tenkanSen)-1] < kijunSen[len(kijunSen)-1] && price < cloudBottom && chikouSpan[len(chikouSpan)-s.Displacement] < price {
			//} else if price < cloudTop {
		} else if tenkanSen[len(tenkanSen)-1] < kijunSen[len(kijunSen)-1] && price < cloudBottom && s.AutoTrade {
			//log.Infof("sell signal")
			balance, _ := s.session.Account.Balance("BTC")
			amount := k.Close.Mul(balance.Available)
			if balance.Available.Compare(0) > 0 && amount.Compare(minTradeAmount) > 0 {
				log.Infof("%v ichimoku cloud price %v cloudtop %v cloudBottom%v, conversion line %v baseline  %v\n", k.StartTime, price, cloudTop, cloudBottom, tenkanSen[len(tenkanSen)-1], kijunSen[len(kijunSen)-1])
				s.placeOrder(ctx, types.SideTypeSell, balance.Available, k.Symbol)
				//log.Infof("sell USDT with %v BTC in %v at %v", balance.Available, k.Close, k.EndTime)
				s.Notify("sell USDT with %v BTC in %v at %v", balance.Available, k.Close, k.StartTime.Time())
			}
			s.Notify("Sell %v signal at %v", k.Symbol, k.Close)
		}

	})
	return nil
}

func highestHigh(candles []types.KLine) float64 {
	high := candles[0].High
	for _, candle := range candles {
		if candle.High.Compare(high) > 0 {
			high = candle.High
		}
	}
	return high.Float64()
}

func lowestLow(candles []types.KLine) float64 {
	low := candles[0].Low
	for _, candle := range candles {
		if candle.Low.Compare(low) < 0 {
			low = candle.Low
		}
	}
	return low.Float64()
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func init() {
	bbgo.RegisterStrategy(ID, &Strategy{})
}
func (s *Strategy) placeOrder(
	ctx context.Context, side types.SideType, quantity fixedpoint.Value, symbol string,
) error {
	market, _ := s.session.Market(symbol)
	_, err := s.orderExecutor.SubmitOrders(ctx, types.SubmitOrder{
		Symbol:   symbol,
		Market:   market,
		Side:     side,
		Type:     types.OrderTypeMarket,
		Quantity: quantity,
		Tag:      "ichimoku_cloud",
	})
	if err != nil {
		log.WithError(err).Errorf("can not place market order")
	}
	return err
}
