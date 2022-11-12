package gecgosio

import "reflect"

type IEventEmitter interface {
	On(evt string, listener interface{}) IEventEmitter
	Off(e string, listener interface{}) bool
	Emit(evt string, args ...interface{}) bool
}

type EventEmitter struct {
	eventListeners map[string][]interface{}
	//mu sync.RWMutex
}

func CreateEventEmitter() IEventEmitter {
	em := &EventEmitter{
		eventListeners: make(map[string][]interface{}),
	}

	return em
}

func (em *EventEmitter) On(e string, listener interface{}) IEventEmitter {
	em.eventListeners[e] = append(em.eventListeners[e], listener)
	return em
}

func (em *EventEmitter) Off(e string, listener interface{}) bool {
	if em.eventListeners != nil {
		for i, evt := range em.eventListeners[e] {
			if reflect.ValueOf(evt) == reflect.ValueOf(listener) {
				em.eventListeners[e][i] = em.eventListeners[e][len(em.eventListeners[e])-1]
				em.eventListeners[e] = em.eventListeners[e][:len(em.eventListeners[e])-1]
				return true
			}
		}
	}

	return false
}

func (em *EventEmitter) Emit(evt string, args ...interface{}) bool {
	//em.mu.Lock()
	listeners := em.eventListeners[evt][:]
	//em.mu.Unlock()

	for _, listener := range listeners {
		l := reflect.ValueOf(listener)
		rargs := make([]reflect.Value, len(args))
		for i, a := range args {
			rargs[i] = reflect.ValueOf(a)
		}
		l.Call(rargs)
	}

	return len(listeners) > 0
}
