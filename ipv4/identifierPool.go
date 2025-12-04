package ipv4

import (
	"errors"
	"time"
)

const ttlSecs = 0x1
const idPoolSize = 1 << 16

type IdentifierPoolId struct {
	destinationAddress uint32
}

type IdentifierSatellite struct {
	insertionUnixSecs int64
}

type IdentifierPool struct {
	length    int
	li, ri    uint16
	activeIds [idPoolSize]*IdentifierSatellite
}

func (idPool *IdentifierPool) empty() bool {
	return idPool.length == 0
}

func (idPool *IdentifierPool) full() bool {
	return idPool.length == idPoolSize
}

func (idPool *IdentifierPool) push(id *IdentifierSatellite) error {
	if idPool.full() {
		return errors.New("queue full")
	}

	idPool.activeIds[idPool.ri] = id
	idPool.ri++
	idPool.length++

	return nil
}

func (idPool *IdentifierPool) peek() *IdentifierSatellite {
	if idPool.empty() {
		return nil
	}

	return idPool.activeIds[idPool.li]
}

func (idPool *IdentifierPool) pop() *IdentifierSatellite {
	if idPool.empty() {
		return nil
	}

	oli := idPool.li
	idPool.li++
	idPool.length--

	return idPool.activeIds[oli]
}

func (idPool *IdentifierPool) GetNextId() uint16 {
	if idPool.full() {
		first := idPool.peek()
		now := time.Now().Unix()

		firstInsertionTime := first.insertionUnixSecs
		timeToWaitUntilExpiration := ttlSecs - (now - firstInsertionTime)
		first.insertionUnixSecs = now - ttlSecs - 1 // ensure ID removed in case of time inconsistencies
		time.Sleep(time.Second * time.Duration(timeToWaitUntilExpiration))
	}

	for !idPool.empty() {
		item := idPool.peek()
		timeToWait := ttlSecs - (time.Now().Unix() - item.insertionUnixSecs)

		if timeToWait > 0 {
			break
		}

		idPool.pop()
	}

	id := idPool.ri
	idPool.push(&IdentifierSatellite{insertionUnixSecs: time.Now().Unix()})

	return id
}
