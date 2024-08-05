package alert

import (
	"context"
	"fmt"
	"time"

	"github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/types"
	"github.com/sirupsen/logrus"
)

const ID = "alert"

var log = logrus.WithField("strategy", ID)

type Strategy struct {
	Symbols  []string       `json:"symbol"`
	Interval types.Interval `json:"interval"`
	Prices   map[string][]types.KLine
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
		session.Subscribe(types.KLineChannel, symbol, types.SubscribeOptions{Interval: "5m"})
		s.fetchHistoricalData(ctx, symbol, session)
	}
}

func (s *Strategy) checkPriceChange(symbol string) {
	prices := s.Prices[symbol]
	if len(prices) < 24 {
		return
	}

	initialPrice := prices[0].Close
	currentPrice := prices[len(prices)-1].Close
	priceChange := ((currentPrice - initialPrice) / initialPrice) * 100

	if priceChange > 30 {
		msg := fmt.Sprintf("Price of %s has increased by more than 30%% in the past 24 hours. Current price: %.2f", symbol, currentPrice.Float64())
		bbgo.Notify(msg)
	}
}
func (s *Strategy) fetchHistoricalData(ctx context.Context, symbol string, session *bbgo.ExchangeSession) {
	endTime := time.Now()
	startTime := endTime.Add(-24 * time.Hour)

	kLines, err := session.Exchange.QueryKLines(ctx, symbol, types.Interval5m, types.KLineQueryOptions{StartTime: &startTime, EndTime: &endTime})
	if err != nil {
		log.Printf("failed to fetch historical data for %s: %v", symbol, err)
		return
	}
	fmt.Println(kLines)
}

func (s *Strategy) Run(ctx context.Context, _ bbgo.OrderExecutor, session *bbgo.ExchangeSession) error {
	s.Prices = make(map[string][]types.KLine)

	s.Subscribe(ctx, session)

	session.MarketDataStream.OnKLineClosed(func(kline types.KLine) {
		log.Infof("catchUp mode is enabled, updating grid orders...")
		s.Prices[kline.Symbol] = append(s.Prices[kline.Symbol], kline)
		if len(s.Prices[kline.Symbol]) > 288 {
			s.Prices[kline.Symbol] = s.Prices[kline.Symbol][1:]
		}
		s.checkPriceChange(kline.Symbol)
	})
	return nil

}
