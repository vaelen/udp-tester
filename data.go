/*
  Copyright (c) 2019 Andrew Young.  All Rights Reserved.

  This file is part of UDP Tester.

  UDP Tester is free software: you can redistribute it and/or modify
  it under the terms of the GNU General Public License as published by
  the Free Software Foundation, either version 3 of the License, or
  (at your option) any later version.

  UDP Tester is distributed in the hope that it will be useful,
  but WITHOUT ANY WARRANTY; without even the implied warranty of
  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
  GNU General Public License for more details.

  You should have received a copy of the GNU General Public License
  along with UDP Tester.  If not, see <https://www.gnu.org/licenses/>.
*/

package main

import (
	"io"
	"log"
	"net"
)

// A ReceivedData struct is returned when data is received
type ReceivedData struct {
	Data    []byte
	Address net.Addr
	Error   error
}

// A DataClient sends and receives data over UDP.
type DataClient struct {
	remoteAddress *net.UDPAddr
	localAddress  string
	connection    net.PacketConn
	received      chan ReceivedData
}

// NewDataClient creates a new DataClient instance
func NewDataClient(address string, localAddress string) (*DataClient, error) {
	remoteAddress, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenPacket("udp", localAddress)
	if err != nil {
		return nil, err
	}
	client := &DataClient{
		remoteAddress: remoteAddress,
		localAddress:  localAddress,
		connection:    conn,
		received:      make(chan ReceivedData),
	}
	go client.readLoop(conn)
	return client, nil
}

func (client *DataClient) readLoop(conn net.PacketConn) {
	defer func() {
		e := recover()
		if e != nil {
			err, ok := e.(error)
			if ok && err.Error() == "send on closed channel" {
				// This happens sometimes when closing the client and is not actually an error.
				return
			}
			log.Printf("Caught panic in readLoop(): %v", e)
		}
	}()
	buf := make([]byte, 65536)
	for {
		n, addr, err := client.connection.ReadFrom(buf)
		r := ReceivedData{
			Data:    make([]byte, n),
			Address: addr,
			Error:   err,
		}
		copy(r.Data, buf)
		client.received <- r
		if err == io.EOF {
			return
		}
	}
}

// Close shuts down the DataClient
func (client *DataClient) Close() error {
	client.connection.Close()
	close(client.received)
	return nil
}

// Send sends the given data to the device
func (client *DataClient) Send(data []byte) error {
	for len(data) > 0 {
		bytesWritten, err := client.connection.WriteTo(data, client.remoteAddress)
		if err != nil {
			return err
		}
		data = data[bytesWritten:]
	}
	return nil
}

// Receive returns a channel that can be used to receive data from the device
func (client *DataClient) Receive() chan ReceivedData {
	return client.received
}
