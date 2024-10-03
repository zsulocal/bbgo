package bbgo

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/c9s/bbgo/pkg/exchange/retry"
	"github.com/c9s/bbgo/pkg/sigchan"
	"github.com/c9s/bbgo/pkg/types"
)

const DefaultCancelOrderWaitTime = 20 * time.Millisecond
const DefaultOrderCancelTimeout = 5 * time.Second

// ActiveOrderBook manages the local active order books.
//
//go:generate callbackgen -type ActiveOrderBook
type ActiveOrderBook struct {
	Symbol string
	orders *types.SyncOrderMap

	newCallbacks      []func(o types.Order)
	filledCallbacks   []func(o types.Order)
	canceledCallbacks []func(o types.Order)

	pendingOrderUpdates *types.SyncOrderMap

	// sig is the order update signal
	// this signal will be emitted when a new order is added or removed.
	C sigchan.Chan

	mu sync.Mutex

	cancelOrderWaitTime time.Duration
	cancelOrderTimeout  time.Duration
}

func NewActiveOrderBook(symbol string) *ActiveOrderBook {
	return &ActiveOrderBook{
		Symbol:              symbol,
		orders:              types.NewSyncOrderMap(),
		pendingOrderUpdates: types.NewSyncOrderMap(),
		C:                   sigchan.New(1),
		cancelOrderWaitTime: DefaultCancelOrderWaitTime,
		cancelOrderTimeout:  DefaultOrderCancelTimeout,
	}
}

func (b *ActiveOrderBook) SetCancelOrderWaitTime(duration time.Duration) {
	b.cancelOrderWaitTime = duration
}

func (b *ActiveOrderBook) MarshalJSON() ([]byte, error) {
	orders := b.Backup()
	return json.Marshal(orders)
}

func (b *ActiveOrderBook) Backup() []types.SubmitOrder {
	return b.orders.Backup()
}

func (b *ActiveOrderBook) BindStream(stream types.Stream) {
	stream.OnOrderUpdate(b.orderUpdateHandler)
}

func (b *ActiveOrderBook) waitClear(
	ctx context.Context, order types.Order, waitTime, timeout time.Duration,
) (bool, error) {
	if !b.orders.Exists(order.OrderID) {
		return true, nil
	}

	timeoutC := time.After(timeout)
	for {
		select {
		case <-time.After(waitTime):
		case <-b.C:
		}

		clear := !b.orders.Exists(order.OrderID)

		select {
		case <-timeoutC:
			return clear, nil

		case <-ctx.Done():
			return clear, ctx.Err()

		default:
			if clear {
				return clear, nil
			}
		}
	}
}

// waitAllClear waits for the order book be clear (meaning every order is removed)
// if err != nil, it's the context error.
func (b *ActiveOrderBook) waitAllClear(ctx context.Context, waitTime, timeout time.Duration) (bool, error) {
	clear := b.NumOfOrders() == 0
	if clear {
		return clear, nil
	}

	timeoutC := time.After(timeout)
	for {
		select {
		case <-time.After(waitTime):
		case <-b.C:
		}

		// update clear flag
		clear = b.NumOfOrders() == 0

		select {
		case <-timeoutC:
			return clear, nil

		case <-ctx.Done():
			return clear, ctx.Err()

		default:
			if clear {
				return clear, nil
			}
		}
	}
}

// FastCancel cancels the orders without verification
// It calls the exchange cancel order api and then remove the orders from the active orderbook directly.
func (b *ActiveOrderBook) FastCancel(ctx context.Context, ex types.Exchange, orders ...types.Order) error {
	// if no orders are given, set to cancelAll
	hasSymbol := b.Symbol != ""
	if len(orders) == 0 {
		orders = b.Orders()
	} else {
		// simple check on given input
		for _, o := range orders {
			if hasSymbol && o.Symbol != b.Symbol {
				return errors.New("[ActiveOrderBook] cancel " + b.Symbol + " orderbook with different order symbol: " + o.Symbol)
			}
		}
	}

	// optimize order cancel for back-testing
	if IsBackTesting {
		return ex.CancelOrders(ctx, orders...)
	}

	log.Debugf("[ActiveOrderBook] no wait cancelling %s orders...", b.Symbol)
	if err := ex.CancelOrders(ctx, orders...); err != nil {
		log.WithError(err).Errorf("[ActiveOrderBook] no wait can not cancel %s orders", b.Symbol)
	}

	for _, o := range orders {
		b.orders.Remove(o.OrderID)
	}
	return nil
}

// GracefulCancel cancels the active orders gracefully
func (b *ActiveOrderBook) GracefulCancel(ctx context.Context, ex types.Exchange, specifiedOrders ...types.Order) error {
	cancelAll := false
	orders := specifiedOrders

	// if no orders are given, set to cancelAll
	if len(specifiedOrders) == 0 {
		orders = b.Orders()
		cancelAll = true
	} else {
		// simple check on given input
		hasSymbol := b.Symbol != ""
		for _, o := range orders {
			if hasSymbol && o.Symbol != b.Symbol {
				return fmt.Errorf("[ActiveOrderBook] canceling %s orderbook with different symbol: %s", b.Symbol, o.Symbol)
			}
		}
	}

	// optimize order cancel for back-testing
	if IsBackTesting {
		return ex.CancelOrders(context.Background(), orders...)
	}

	log.Debugf("[ActiveOrderBook] gracefully cancelling %s orders...", b.Symbol)
	waitTime := b.cancelOrderWaitTime

	startTime := time.Now()
	// ensure every order is canceled
	for {
		// Some orders in the variable are not created on the server side yet,
		// If we cancel these orders directly, we will get an unsent order error
		// We wait here for a while for server to create these orders.
		// time.Sleep(SentOrderWaitTime)

		// since ctx might be canceled, we should use background context here
		if err := ex.CancelOrders(context.Background(), orders...); err != nil {
			log.WithError(err).Warnf("[ActiveOrderBook] can not cancel %d %s orders", len(orders), b.Symbol)
		}

		log.Debugf("[ActiveOrderBook] waiting %s for %d %s orders to be cancelled...", waitTime, len(orders), b.Symbol)

		if cancelAll {
			clear, err := b.waitAllClear(ctx, waitTime, b.cancelOrderTimeout)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					log.WithError(err).Errorf("order cancel error")
				}

				break
			}

			if clear {
				log.Debugf("[ActiveOrderBook] %d %s orders are canceled", len(orders), b.Symbol)
				break
			}

			log.Warnf("[ActiveOrderBook] %d/%d %s orders are not cancelled yet", b.NumOfOrders(), len(orders), b.Symbol)
			b.Print()

		} else {
			existingOrders := b.filterExistingOrders(orders)
			if len(existingOrders) == 0 {
				log.Debugf("[ActiveOrderBook] orders are canceled")
				break
			}
		}

		// verify the current open orders via the RESTful API
		if orderQueryService, ok := ex.(types.ExchangeOrderQueryService); ok {
			for idx, o := range orders {
				retOrder, err := retry.QueryOrderUntilSuccessful(ctx, orderQueryService, types.OrderQuery{
					Symbol:  o.Symbol,
					OrderID: strconv.FormatUint(o.OrderID, 10),
				})

				if err != nil {
					log.WithError(err).Errorf("unable to update order #%d", o.OrderID)
					continue
				} else if retOrder != nil {
					b.Update(*retOrder)

					orders[idx] = *retOrder
				}
			}

			if cancelAll {
				orders = b.Orders()
			} else {
				// for partial cancel
				orders = filterCanceledOrders(orders)
			}
		} else {
			log.Warnf("[ActiveOrderBook] using open orders API to verify the active orders...")

			var symbolOrdersMap = categorizeOrderBySymbol(orders)
			var errOccurred bool
			var leftOrders types.OrderSlice
			for symbol, symbolOrders := range symbolOrdersMap {
				openOrders, err := ex.QueryOpenOrders(ctx, symbol)
				if err != nil {
					errOccurred = true
					log.WithError(err).Errorf("can not query %s open orders", symbol)
					break
				}

				openOrderMap := types.NewOrderMap(openOrders...)
				for _, o := range symbolOrders {
					// if it's not on the order book (open orders),
					// we should remove it from our local side
					if !openOrderMap.Exists(o.OrderID) {
						b.Remove(o)
					} else {
						leftOrders.Add(o)
					}
				}
			}

			// if an error occurs, we cannot update the orders because it will result in an empty order slice.
			if !errOccurred {
				// update order slice for the next try
				orders = leftOrders
			}

		}

	}

	log.Debugf("[ActiveOrderBook] all %s orders are cancelled successfully in %s", b.Symbol, time.Since(startTime))
	return nil
}

func (b *ActiveOrderBook) orderUpdateHandler(order types.Order) {
	if oldOrder, ok := b.Get(order.OrderID); ok {
		order.Tag = oldOrder.Tag
		order.GroupID = oldOrder.GroupID
	}
	b.Update(order)
}

func (b *ActiveOrderBook) Print() {
	orders := b.orders.Orders()
	orders = types.SortOrdersByPrice(orders, true)
	orders.Print()
}

// Update updates the order by the order status and emit the related events.
// When order is filled, the order will be removed from the internal order storage.
// When order is New or PartiallyFilled, the internal order will be updated according to the latest order update.
// When the order is cancelled, it will be removed from the internal order storage.
func (b *ActiveOrderBook) Update(order types.Order) {
	hasSymbol := len(b.Symbol) > 0
	if hasSymbol && order.Symbol != b.Symbol {
		return
	}

	b.mu.Lock()
	if !b.orders.Exists(order.OrderID) {
		log.Debugf("[ActiveOrderBook] order #%d %s does not exist, adding it to pending order update", order.OrderID, order.Status)
		b.pendingOrderUpdates.Add(order)
		b.mu.Unlock()
		return
	}

	// if order update time is too old, skip it
	if previousOrder, ok := b.orders.Get(order.OrderID); ok {
		// the arguments ordering is important here
		// if we can't detect which is newer, isNewerOrderUpdate returns false
		// if you pass two same objects to isNewerOrderUpdate, it returns false
		if !isNewerOrderUpdate(order, previousOrder) {
			log.Infof("[ActiveOrderBook] order #%d updateTime %s is out of date, skip it", order.OrderID, order.UpdateTime)
			b.mu.Unlock()
			return
		}
	}

	switch order.Status {
	case types.OrderStatusFilled:
		// make sure we have the order and we remove it
		removed := b.orders.Remove(order.OrderID)
		b.mu.Unlock()

		if removed {
			log.Infof("[ActiveOrderBook] order #%d is filled: %s", order.OrderID, order.String())
			b.EmitFilled(order)
		}
		b.C.Emit()

	case types.OrderStatusPartiallyFilled:
		b.orders.Update(order)
		b.mu.Unlock()

	case types.OrderStatusNew:
		b.orders.Update(order)
		b.mu.Unlock()

		b.C.Emit()

	case types.OrderStatusCanceled, types.OrderStatusRejected:
		// TODO: note that orders transit to "canceled" may have partially filled
		log.Debugf("[ActiveOrderBook] order is %s, removing order %s", order.Status, order)
		b.orders.Remove(order.OrderID)
		b.mu.Unlock()

		if order.Status == types.OrderStatusCanceled {
			b.EmitCanceled(order)
		}
		b.C.Emit()

	default:
		b.mu.Unlock()
		log.Warnf("[ActiveOrderBook] unhandled order status: %s", order.Status)
	}
}

func (b *ActiveOrderBook) Add(orders ...types.Order) {
	hasSymbol := len(b.Symbol) > 0

	for _, order := range orders {
		if hasSymbol && b.Symbol != order.Symbol {
			continue
		}

		b.add(order)
	}
}

func isNewerOrderUpdate(a, b types.Order) bool {
	// compare state first
	switch a.Status {

	case types.OrderStatusCanceled, types.OrderStatusRejected: // canceled is a final state
		switch b.Status {
		case types.OrderStatusNew, types.OrderStatusPartiallyFilled:
			return true
		}

	case types.OrderStatusPartiallyFilled:
		switch b.Status {
		case types.OrderStatusNew:
			return true
		case types.OrderStatusPartiallyFilled:
			// unknown for equal
			if a.ExecutedQuantity.Compare(b.ExecutedQuantity) > 0 {
				return true
			}

		}

	case types.OrderStatusFilled:
		switch b.Status {
		case types.OrderStatusFilled, types.OrderStatusPartiallyFilled, types.OrderStatusNew:
			return true
		}
	}

	return isNewerOrderUpdateTime(a, b)
}

func isNewerOrderUpdateTime(a, b types.Order) bool {
	au := time.Time(a.UpdateTime)
	bu := time.Time(b.UpdateTime)

	if !au.IsZero() && !bu.IsZero() && au.After(bu) {
		return true
	}

	if !au.IsZero() && bu.IsZero() {
		return true
	}

	return false
}

// add the order to the active order book and check the pending order
func (b *ActiveOrderBook) add(order types.Order) {
	if pendingOrder, ok := b.pendingOrderUpdates.Get(order.OrderID); ok {
		// if the pending order update time is newer than the adding order
		// we should use the pending order rather than the adding order.
		// if the pending order is older, then we should add the new one, and drop the pending order
		log.Debugf("found pending order update: %+v", pendingOrder)
		if isNewerOrderUpdate(pendingOrder, order) {
			log.Debugf("pending order update is newer: %+v", pendingOrder)
			order = pendingOrder
		}

		b.orders.Add(order)
		b.pendingOrderUpdates.Remove(pendingOrder.OrderID)
		b.EmitNew(order)

		// when using add(order), it's usually a new maker order on the order book.
		// so, when it's not status=new, we should trigger order update handler
		if order.Status != types.OrderStatusNew {
			// emit the order update handle function to trigger callback
			b.Update(order)
		}

	} else {
		b.orders.Add(order)
		b.EmitNew(order)
	}
}

func (b *ActiveOrderBook) Exists(order types.Order) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.orders.Exists(order.OrderID)
}

func (b *ActiveOrderBook) Get(orderID uint64) (types.Order, bool) {
	return b.orders.Get(orderID)
}

func (b *ActiveOrderBook) Remove(order types.Order) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.orders.Remove(order.OrderID)
}

func (b *ActiveOrderBook) NumOfOrders() int {
	return b.orders.Len()
}

func (b *ActiveOrderBook) Orders() types.OrderSlice {
	return b.orders.Orders()
}

func (b *ActiveOrderBook) Lookup(f func(o types.Order) bool) *types.Order {
	return b.orders.Lookup(f)
}

func (b *ActiveOrderBook) filterExistingOrders(orders []types.Order) (existingOrders types.OrderSlice) {
	for _, o := range orders {
		// skip market order
		// this prevents if someone added a market order to the active order book
		if o.Type == types.OrderTypeMarket {
			continue
		}

		if b.Exists(o) {
			existingOrders.Add(o)
		}
	}

	return existingOrders
}

func categorizeOrderBySymbol(orders types.OrderSlice) map[string]types.OrderSlice {
	orderMap := map[string]types.OrderSlice{}

	for _, order := range orders {
		orderMap[order.Symbol] = append(orderMap[order.Symbol], order)
	}

	return orderMap
}

func filterCanceledOrders(orders types.OrderSlice) (ret types.OrderSlice) {
	for _, o := range orders {
		if o.Status == types.OrderStatusCanceled {
			continue
		}

		ret = append(ret, o)
	}

	return ret
}
