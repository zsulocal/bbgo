package alert

import (
	"context"
	"fmt"
	"time"

	"github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	"github.com/sirupsen/logrus"
)

const ID = "alert"

var log = logrus.WithField("strategy", ID)

type Strategy struct {
	Symbols  []string       `json:"symbol"`
	Interval types.Interval `json:"interval"`
	Prices   map[string][]types.KLine
	Change   fixedpoint.Value `json:"change"`
}

func init() {
	// Register our struct type to BBGO
	// Note that you don't need to field the fields.
	// BBGO uses reflect to parse your type information.
	bbgo.RegisterStrategy(ID, &Strategy{})
}

func (s *Strategy) ID() string {
	return ID
}

func (s *Strategy) Subscribe(ctx context.Context, session *bbgo.ExchangeSession) {
	for _, symbol := range s.Symbols {
		session.Subscribe(types.KLineChannel, symbol, types.SubscribeOptions{Interval: s.Interval})
		s.fetchHistoricalData(ctx, symbol, session)
	}
}

func (s *Strategy) checkPriceChange(symbol string) {
	prices := s.Prices[symbol]
	if len(prices) < 20 {
		return
	}

	initialPrice := prices[0].Close
	currentPrice := prices[len(prices)-1].Close
	priceChange := ((currentPrice.Float64() - initialPrice.Float64()) / initialPrice.Float64())
	log.Infof("price change %.2f %.2f %.4f", initialPrice.Float64(), currentPrice.Float64(), priceChange)

	if priceChange > s.Change.Float64() {
		msg := fmt.Sprintf("Price of %s has increased by more than 30%% in the past 24 hours. Current price: %.2f", symbol, currentPrice.Float64())
		bbgo.Notify(msg)
	}
}
func (s *Strategy) fetchHistoricalData(ctx context.Context, symbol string, session *bbgo.ExchangeSession) {
	endTime := time.Now()
	startTime := endTime.Add(-24 * time.Hour)

	kLines, err := session.Exchange.QueryKLines(ctx, symbol, s.Interval, types.KLineQueryOptions{StartTime: &startTime, EndTime: &endTime})
	if err != nil {
		log.Printf("failed to fetch historical data for %s: %v", symbol, err)
		return
	}
	s.Prices[symbol] = kLines
}

func (s *Strategy) Run(ctx context.Context, _ bbgo.OrderExecutor, session *bbgo.ExchangeSession) error {
	s.Prices = make(map[string][]types.KLine)

	s.Subscribe(ctx, session)

	session.MarketDataStream.OnKLineClosed(func(kline types.KLine) {
		log.Infof("catchUp mode is enabled, updating grid orders...")
		s.Prices[kline.Symbol] = append(s.Prices[kline.Symbol], kline)
		if len(s.Prices[kline.Symbol]) > 20 {
			s.Prices[kline.Symbol] = s.Prices[kline.Symbol][1:]
		}
		log.Printf("%s now: %.5f %.5f", kline.Symbol, kline.Close.Float64(), s.Prices[kline.Symbol][0].Open.Float64())
		s.checkPriceChange(kline.Symbol)
	})
	return nil

}
