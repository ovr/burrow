// Copyright 2017 Monax Industries Limited
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package event

import (
	"encoding/hex"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var mockInterval = 20 * time.Millisecond

type mockSub struct {
	subId   string
	eventId string
	f       func(AnyEventData)
	sdChan  chan struct{}
}

type mockEventData struct {
	subId   string
	eventId string
}

func (eventData mockEventData) AssertIsEVMEventData() {}

// A mock event
func newMockSub(subId, eventId string, f func(AnyEventData)) mockSub {
	return mockSub{subId, eventId, f, make(chan struct{})}
}

type mockEventEmitter struct {
	subs  map[string]mockSub
	mutex *sync.Mutex
}

func newMockEventEmitter() *mockEventEmitter {
	return &mockEventEmitter{make(map[string]mockSub), &sync.Mutex{}}
}

func (mee *mockEventEmitter) Subscribe(subId, eventId string, callback func(AnyEventData)) error {
	if _, ok := mee.subs[subId]; ok {
		return nil
	}
	me := newMockSub(subId, eventId, callback)
	mee.mutex.Lock()
	mee.subs[subId] = me
	mee.mutex.Unlock()

	go func() {
		for {
			select {
			case <-me.sdChan:
				mee.mutex.Lock()
				delete(mee.subs, subId)
				mee.mutex.Unlock()
				return
			case <-time.After(mockInterval):
				me.f(AnyEventData{BurrowEventData: &EventData{
					EventDataInner: mockEventData{subId: subId, eventId: eventId},
				}})
			}
		}
	}()
	return nil
}

func (mee *mockEventEmitter) Unsubscribe(subId string) error {
	mee.mutex.Lock()
	sub, ok := mee.subs[subId]
	mee.mutex.Unlock()
	if !ok {
		return nil
	}
	sub.sdChan <- struct{}{}
	return nil
}

// Test that event subscriptions can be added manually and then automatically reaped.
func TestSubReaping(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	NUM_SUBS := 100
	reaperThreshold = 200 * time.Millisecond
	reaperTimeout = 100 * time.Millisecond

	mee := newMockEventEmitter()
	eSubs := NewSubscriptions(mee)
	doneChan := make(chan error)
	go func() {
		for i := 0; i < NUM_SUBS; i++ {
			time.Sleep(2 * time.Millisecond)
			go func() {
				id, err := eSubs.Add("WeirdEvent")
				if err != nil {
					doneChan <- err
					return
				}
				if len(id) != 64 {
					doneChan <- fmt.Errorf("Id not of length 64")
					return
				}
				_, err2 := hex.DecodeString(id)
				if err2 != nil {
					doneChan <- err2
				}

				doneChan <- nil
			}()
		}
	}()
	k := 0
	for k < NUM_SUBS {
		err := <-doneChan
		assert.NoError(t, err)
		k++
	}
	time.Sleep(1100 * time.Millisecond)

	assert.Len(t, mee.subs, 0)
	assert.Len(t, eSubs.subs, 0)
	t.Logf("Added %d subs that were all automatically reaped.", NUM_SUBS)
}

// Test that event subscriptions can be added and removed manually.
func TestSubManualClose(t *testing.T) {
	NUM_SUBS := 100
	// Keep the reaper out of this.
	reaperThreshold = 10000 * time.Millisecond
	reaperTimeout = 10000 * time.Millisecond

	mee := newMockEventEmitter()
	eSubs := NewSubscriptions(mee)
	doneChan := make(chan error)
	go func() {
		for i := 0; i < NUM_SUBS; i++ {
			time.Sleep(2 * time.Millisecond)
			go func() {
				id, err := eSubs.Add("WeirdEvent")
				if err != nil {
					doneChan <- err
					return
				}
				if len(id) != 64 {
					doneChan <- fmt.Errorf("Id not of length 64")
					return
				}
				_, err2 := hex.DecodeString(id)
				if err2 != nil {
					doneChan <- err2
				}
				time.Sleep(100 * time.Millisecond)
				err3 := eSubs.Remove(id)
				if err3 != nil {
					doneChan <- err3
				}
				doneChan <- nil
			}()
		}
	}()
	k := 0
	for k < NUM_SUBS {
		err := <-doneChan
		assert.NoError(t, err)
		k++
	}

	assert.Len(t, eSubs.subs, 0)
	t.Logf("Added %d subs that were all closed down by unsubscribing.", NUM_SUBS)
}

// Test that the system doesn't fail under high pressure.
func TestSubFlooding(t *testing.T) {
	NUM_SUBS := 100
	// Keep the reaper out of this.
	reaperThreshold = 10000 * time.Millisecond
	reaperTimeout = 10000 * time.Millisecond
	// Crank it up. Now pressure is 10 times higher on each sub.
	mockInterval = 1 * time.Millisecond
	mee := newMockEventEmitter()
	eSubs := NewSubscriptions(mee)
	doneChan := make(chan error)
	go func() {
		for i := 0; i < NUM_SUBS; i++ {
			time.Sleep(1 * time.Millisecond)
			go func() {
				id, err := eSubs.Add("WeirdEvent")
				if err != nil {
					doneChan <- err
					return
				}
				if len(id) != 64 {
					doneChan <- fmt.Errorf("Id not of length 64")
					return
				}
				_, err2 := hex.DecodeString(id)
				if err2 != nil {
					doneChan <- err2
				}
				time.Sleep(1000 * time.Millisecond)
				err3 := eSubs.Remove(id)
				if err3 != nil {
					doneChan <- err3
				}
				doneChan <- nil
			}()
		}
	}()
	k := 0
	for k < NUM_SUBS {
		err := <-doneChan
		assert.NoError(t, err)
		k++
	}

	assert.Len(t, eSubs.subs, 0)
	t.Logf("Added %d subs that all received 1000 events each. They were all closed down by unsubscribing.", NUM_SUBS)
}