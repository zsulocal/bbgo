package ichimoku

import (
	"context"
	"fmt"
	"github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/types"
	"github.com/sirupsen/logrus"
)

const ID = "ichimoku_cloud"

var log = logrus.WithField("strategy", ID)

type Strategy struct {
	//	*common.Strategy
	*bbgo.Notifiability
	Environment       *bbgo.Environment
	Market            types.Market
	Symbol            string         `json:"symbol"`
	Interval          types.Interval `json:"interval"`
	ConversionPeriod  int
	BasePeriod        int
	LaggingSpanPeriod int
	Displacement      int
	klines            []types.KLine
}

func (s *Strategy) Subscribe(session *bbgo.ExchangeSession) {
	session.Subscribe(types.KLineChannel, s.Symbol, types.SubscribeOptions{Interval: s.Interval})
}

func (s *Strategy) ID() string {
	return ID
}

func (s *Strategy) InstanceID() string {
	return fmt.Sprintf("%s:%s:%s", ID, s.Symbol, s.Interval)
}

func (s *Strategy) calculateIchimokuCloud() (tenkanSen, kijunSen, senkouSpanA, senkouSpanB, chikouSpan []float64) {
	candles := s.klines
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

	kLineStore, _ := session.MarketDataStore(s.Symbol)

	if klines, ok := kLineStore.KLinesOfInterval(s.Interval); ok {
		s.klines = (*klines)[0:]
	}

	fmt.Println(s.klines)

	session.MarketDataStream.OnKLineClosed(func(k types.KLine) {

		tenkanSen, kijunSen, senkouSpanA, senkouSpanB, chikouSpan := s.calculateIchimokuCloud()
		price := k.Close.Float64()
		cloudTop := max(senkouSpanA[len(senkouSpanA)-s.Displacement], senkouSpanB[len(senkouSpanB)-s.Displacement])
		cloudBottom := min(senkouSpanA[len(senkouSpanA)-s.Displacement], senkouSpanB[len(senkouSpanB)-s.Displacement])

		if tenkanSen[len(tenkanSen)-1] > kijunSen[len(kijunSen)-1] && price > cloudTop && chikouSpan[len(chikouSpan)-s.Displacement] > price {
			log.Info("buy")
		} else if tenkanSen[len(tenkanSen)-1] < kijunSen[len(kijunSen)-1] && price < cloudBottom && chikouSpan[len(chikouSpan)-s.Displacement] < price {
			log.Info("sell")
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
