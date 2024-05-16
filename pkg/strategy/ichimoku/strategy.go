package ichimoku

import (
	"github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/types"
	"log"
)

type IchimokuCloudStrategy struct {
	Market            types.Market
	ConversionPeriod  int
	BasePeriod        int
	LaggingSpanPeriod int
	Displacement      int
	Risk              float64
}

func NewIchimokuCloudStrategy() *IchimokuCloudStrategy {
	return &IchimokuCloudStrategy{
		ConversionPeriod:  9,
		BasePeriod:        26,
		LaggingSpanPeriod: 52,
		Displacement:      26,
		Risk:              0.02,
	}
}

func (s *IchimokuCloudStrategy) ID() string {
	return "ichimoku_cloud"
}

func (s *IchimokuCloudStrategy) OnCandle(candle types.Candle) {
	candles := s.Market.KLineStore.Candles
	tenkanSen, kijunSen, senkouSpanA, senkouSpanB, chikouSpan := s.CalculateIchimokuCloud(candles)

	price := candle.Close
	cloudTop := max(senkouSpanA[len(senkouSpanA)-s.Displacement], senkouSpanB[len(senkouSpanB)-s.Displacement])
	cloudBottom := min(senkouSpanA[len(senkouSpanA)-s.Displacement], senkouSpanB[len(senkouSpanB)-s.Displacement])

	if tenkanSen[len(tenkanSen)-1] > kijunSen[len(kijunSen)-1] && price > cloudTop && chikouSpan[len(chikouSpan)-s.Displacement] > price {
		s.buySignal(candle)
	} else if tenkanSen[len(tenkanSen)-1] < kijunSen[len(kijunSen)-1] && price < cloudBottom && chikouSpan[len(chikouSpan)-s.Displacement] < price {
		s.sellSignal(candle)
	}
}

func (s *IchimokuCloudStrategy) buySignal(candle types.Candle) {
	balance, err := s.Market.GetBalance()
	if err != nil {
		log.Println("Error getting balance:", err)
		return
	}
	riskAmount := balance[s.Market.QuoteCurrency].Available * s.Risk
	orderSize := riskAmount / candle.Close
	s.Market.OrderExecutor.SubmitOrder(types.SubmitOrder{
		Symbol:   s.Market.Symbol,
		Side:     types.SideTypeBuy,
		Type:     types.OrderTypeMarket,
		Quantity: orderSize,
	})
	log.Printf("Buy order placed for %f units at %f", orderSize, candle.Close)
}

func (s *IchimokuCloudStrategy) sellSignal(candle types.Candle) {
	position, err := s.Market.GetPosition()
	if err != nil {
		log.Println("Error getting position:", err)
		return
	}
	s.Market.OrderExecutor.SubmitOrder(types.SubmitOrder{
		Symbol:   s.Market.Symbol,
		Side:     types.SideTypeSell,
		Type:     types.OrderTypeMarket,
		Quantity: position[s.Market.BaseCurrency].Quantity,
	})
	log.Printf("Sell order placed for %f units at %f", position[s.Market.BaseCurrency].Quantity, candle.Close)
}

func (s *IchimokuCloudStrategy) CalculateIchimokuCloud(candles []types.Candle) (tenkanSen, kijunSen, senkouSpanA, senkouSpanB, chikouSpan []float64) {
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
			chikouSpan[i-s.Displacement] = candles[i].Close
		}
	}

	return tenkanSen, kijunSen, senkouSpanA, senkouSpanB, chikouSpan
}

func highestHigh(candles []types.Candle) float64 {
	high := candles[0].High
	for _, candle := range candles {
		if candle.High > high {
			high = candle.High
		}
	}
	return high
}

func lowestLow(candles []types.Candle) float64 {
	low := candles[0].Low
	for _, candle := range candles {
		if candle.Low < low {
			low = candle.Low
		}
	}
	return low
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
