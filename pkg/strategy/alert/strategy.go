package alert

import (
	"context"
	"time"

	"github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	"github.com/sirupsen/logrus"
)

const ID = "alert"

var log = logrus.WithField("strategy", ID)

type Strategy struct {
	Change   fixedpoint.Value `json:"change"` // 价格变化百分比
	Notified map[string]time.Time
	Prices   map[string]types.BookTicker
}

func init() {
	// Register our struct type to BBGO
	bbgo.RegisterStrategy(ID, &Strategy{})
}

func (s *Strategy) ID() string {
	return ID
}

func (s *Strategy) Subscribe(ctx context.Context, session *bbgo.ExchangeSession) {
	session.Subscribe(types.BookTickerChannel, "*", types.SubscribeOptions{})
}

func (s *Strategy) checkPriceChange(symbol string, ticker types.BookTicker) {
	/*
		// 检查是否需要发送通知
		if last, ok := s.Notified[symbol]; ok {
			if last.Add(time.Minute * 30).After(time.Now()) {
				return
			}
		}

		initialTicker, exists := s.Prices[symbol]
		if !exists {
			// 如果没有初始价格记录，则记录当前价格
			s.Prices[symbol] = ticker
			return
		}
		init

		initialPrice := initialTicker.Last
		currentPrice := ticker.Last
		priceChange := (currentPrice.Float64() - initialPrice.Float64()) / initialPrice.Float64()
		log.Infof("price change for %s: %.2f -> %.2f = %.4f", symbol, initialPrice.Float64(), currentPrice.Float64(), priceChange)

		if priceChange > s.Change.Float64() {
			msg := fmt.Sprintf("Price of %s has increased by more than %.2f%%. Current price: %.2f", symbol, s.Change.Float64()*100, currentPrice.Float64())
			bbgo.Notify(msg)
			s.Notified[symbol] = time.Now()
			// 更新初始价格为当前价格
			s.Prices[symbol] = ticker
		}
	*/
}

func (s *Strategy) Run(ctx context.Context, _ bbgo.OrderExecutor, session *bbgo.ExchangeSession) error {
	s.Prices = make(map[string]types.BookTicker)
	s.Notified = make(map[string]time.Time)

	s.Subscribe(ctx, session)

	session.MarketDataStream.OnBookTickerUpdate(func(ticker types.BookTicker) {
		s.checkPriceChange(ticker.Symbol, ticker)
	})

	return nil
}
