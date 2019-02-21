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

// Package main contains a standalone app that uses the UDP data client that is part of Star Receiver for testing the data connection between StarPass and a network based modem.
package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/rivo/tview"
)

var delayBetweenCommands = time.Millisecond * 100
var delayAfterCommands = time.Millisecond * 500

const toMbps = 8.0 / 1.0e6
const rateFormat = "%0.3f Mbps"

type testInfoUI struct {
	ctx             context.Context
	cancel          context.CancelFunc
	app             *tview.Application
	panel           *tview.Flex
	testType        *tview.TextView
	txAddress       *tview.TableCell
	txSent          *tview.TableCell
	txDataRate      *tview.TableCell
	txDataSize      *tview.TableCell
	txInterval      *tview.TableCell
	rxAddress       *tview.TableCell
	rxReceived      *tview.TableCell
	rxDataRate      *tview.TableCell
	rxSizeErrors    *tview.TableCell
	rxDataErrors    *tview.TableCell
	sentPackets     uint64
	receivedPackets uint64
	sizeErrors      uint64
	dataErrors      uint64
	sentBytes       int64
	receivedBytes   int64
	lastTS          int64
}

func (ui *testInfoUI) running(testType string) {
	queueUpdateAndDraw(ui.app, func() {
		ui.testType.SetText(fmt.Sprintf("Running %s", testType))
	})
}

func (ui *testInfoUI) finished(testType string) {
	queueUpdateAndDraw(ui.app, func() {
		ui.testType.SetText(fmt.Sprintf("Finished %s", testType))
	})
}

func (ui *testInfoUI) remoteAddress(remoteAddress string) {
	queueUpdateAndDraw(ui.app, func() {
		ui.txAddress.SetText(remoteAddress)
	})
}

func (ui *testInfoUI) localAddress(localAddress string) {
	queueUpdateAndDraw(ui.app, func() {
		ui.rxAddress.SetText(localAddress)
	})
}

func (ui *testInfoUI) interval(interval time.Duration) {
	queueUpdateAndDraw(ui.app, func() {
		ui.txInterval.SetText(fmt.Sprintf("%v", interval))
	})
}

func (ui *testInfoUI) dataSize(dataSize int) {
	queueUpdateAndDraw(ui.app, func() {
		ui.txDataSize.SetText(fmt.Sprintf("%d bytes", dataSize))
	})
}

func queueUpdateAndDraw(app *tview.Application, f func()) {
	app.QueueUpdateDraw(f)
}

func (ui *testInfoUI) sentPacket(size int) {
	atomic.AddUint64(&ui.sentPackets, 1)
	atomic.AddInt64(&ui.sentBytes, int64(size))
}

func (ui *testInfoUI) receivedPacket(size int) {
	atomic.AddUint64(&ui.receivedPackets, 1)
	atomic.AddInt64(&ui.receivedBytes, int64(size))
}

func (ui *testInfoUI) dataError() {
	atomic.AddUint64(&ui.dataErrors, 1)
}

func (ui *testInfoUI) sizeError() {
	atomic.AddUint64(&ui.sizeErrors, 1)
}

func (ui *testInfoUI) reset() {
	atomic.StoreUint64(&ui.sentPackets, 0)
	atomic.StoreUint64(&ui.receivedPackets, 0)
	atomic.StoreInt64(&ui.lastTS, time.Now().UnixNano())
	atomic.StoreInt64(&ui.sentBytes, 0)
	atomic.StoreInt64(&ui.receivedBytes, 0)
	atomic.StoreUint64(&ui.sizeErrors, 0)
	atomic.StoreUint64(&ui.dataErrors, 0)
}

func (ui *testInfoUI) updateRate() {
	currentTS := time.Now().UnixNano()
	bytesSent := atomic.SwapInt64(&ui.sentBytes, 0)
	bytesReceived := atomic.SwapInt64(&ui.receivedBytes, 0)
	lastTS := atomic.SwapInt64(&ui.lastTS, currentTS)

	start := time.Unix(0, lastTS)
	end := time.Unix(0, currentTS)
	duration := end.Sub(start)

	sendRate := float64(bytesSent) / duration.Seconds() * toMbps
	receiveRate := float64(bytesReceived) / duration.Seconds() * toMbps

	queueUpdateAndDraw(ui.app, func() {
		ui.txDataRate.SetText(fmt.Sprintf("%0.3f Mbps", sendRate))
		ui.rxDataRate.SetText(fmt.Sprintf("%0.3f Mbps", receiveRate))
	})
}

func (ui *testInfoUI) updateCounters() {
	sentPackets := atomic.LoadUint64(&ui.sentPackets)
	receivedPackets := atomic.LoadUint64(&ui.receivedPackets)
	sizeErrors := atomic.LoadUint64(&ui.sizeErrors)
	dataErrors := atomic.LoadUint64(&ui.dataErrors)
	queueUpdateAndDraw(ui.app, func() {
		ui.txSent.SetText(fmt.Sprintf("%d", sentPackets))
		ui.rxReceived.SetText(fmt.Sprintf("%d", receivedPackets))
		ui.rxSizeErrors.SetText(fmt.Sprintf("%d", sizeErrors))
		ui.rxDataErrors.SetText(fmt.Sprintf("%d", dataErrors))
	})
}

func createApplication() (app *tview.Application) {
	app = tview.NewApplication()
	pages := tview.NewPages()

	infoUI := createInfoPanel(app)
	logPanel := createTextViewPanel(app, "Log")
	outputPanel := createOutputPanel(app, infoUI.panel, logPanel)

	log.SetOutput(logPanel)

	commandList := createCommandList()
	commandList.AddItem("Listen", "", 'l', listenCommand(pages, infoUI))
	commandList.AddItem("Loopback Test", "", 't', loopbackTestCommand(pages, infoUI))
	commandList.AddItem("Stop Test", "", 's', stopTestCommand(infoUI))
	commandList.AddItem("Quit", "", 'q', app.Stop)

	layout := createMainLayout(commandList, outputPanel)
	pages.AddPage("main", layout, true, true)

	app.SetRoot(pages, true)

	return app

}

func createInfoPanel(app *tview.Application) (infoUI *testInfoUI) {
	///// Info /////
	infoPanel := tview.NewFlex().SetDirection(tview.FlexRow)

	infoUI = &testInfoUI{}
	infoUI.app = app
	infoUI.panel = infoPanel

	infoUI.testType = tview.NewTextView()
	infoUI.testType.SetBorder(true)
	infoUI.testType.SetText("No Test Running")
	infoUI.testType.SetTextAlign(tview.AlignCenter)
	infoPanel.AddItem(infoUI.testType, 3, 1, false)

	txInfo := tview.NewTable()
	txInfo.SetBorder(true).SetTitle("Transmit")

	txInfo.SetCellSimple(0, 0, "Address:")
	txInfo.GetCell(0, 0).SetAlign(tview.AlignRight)
	infoUI.txAddress = tview.NewTableCell("0.0.0.0:0")
	txInfo.SetCell(0, 1, infoUI.txAddress)

	txInfo.SetCellSimple(1, 0, "Sent:")
	txInfo.GetCell(1, 0).SetAlign(tview.AlignRight)
	infoUI.txSent = tview.NewTableCell("0")
	txInfo.SetCell(1, 1, infoUI.txSent)

	txInfo.SetCellSimple(2, 0, "Data Rate:")
	txInfo.GetCell(2, 0).SetAlign(tview.AlignRight)
	infoUI.txDataRate = tview.NewTableCell("0 Mbps")
	txInfo.SetCell(2, 1, infoUI.txDataRate)

	txInfo.SetCellSimple(3, 0, "Data Size:")
	txInfo.GetCell(3, 0).SetAlign(tview.AlignRight)
	infoUI.txDataSize = tview.NewTableCell("0")
	txInfo.SetCell(3, 1, infoUI.txDataSize)

	txInfo.SetCellSimple(4, 0, "Interval:")
	txInfo.GetCell(4, 0).SetAlign(tview.AlignRight)
	infoUI.txInterval = tview.NewTableCell("0")
	txInfo.SetCell(4, 1, infoUI.txInterval)

	rxInfo := tview.NewTable()
	rxInfo.SetBorder(true).SetTitle("Receive")

	rxInfo.SetCellSimple(0, 0, "Address:")
	rxInfo.GetCell(0, 0).SetAlign(tview.AlignRight)
	infoUI.rxAddress = tview.NewTableCell("0.0.0.0:0")
	rxInfo.SetCell(0, 1, infoUI.rxAddress)

	rxInfo.SetCellSimple(1, 0, "Received:")
	rxInfo.GetCell(1, 0).SetAlign(tview.AlignRight)
	infoUI.rxReceived = tview.NewTableCell("0")
	rxInfo.SetCell(1, 1, infoUI.rxReceived)

	rxInfo.SetCellSimple(2, 0, "Data Rate:")
	rxInfo.GetCell(2, 0).SetAlign(tview.AlignRight)
	infoUI.rxDataRate = tview.NewTableCell("0 Mbps")
	rxInfo.SetCell(2, 1, infoUI.rxDataRate)

	rxInfo.SetCellSimple(3, 0, "Size Errors:")
	rxInfo.GetCell(3, 0).SetAlign(tview.AlignRight)
	infoUI.rxSizeErrors = tview.NewTableCell("0")
	rxInfo.SetCell(3, 1, infoUI.rxSizeErrors)

	rxInfo.SetCellSimple(4, 0, "Data Errors:")
	rxInfo.GetCell(4, 0).SetAlign(tview.AlignRight)
	infoUI.rxDataErrors = tview.NewTableCell("0")
	rxInfo.SetCell(4, 1, infoUI.rxDataErrors)

	infoInnerPanel := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(txInfo, 0, 1, false).
		AddItem(rxInfo, 0, 1, false)

	infoPanel.AddItem(infoInnerPanel, 0, 1, false)

	return infoUI
}

func createTextViewPanel(app *tview.Application, name string) (panel *tview.TextView) {
	panel = tview.NewTextView()
	panel.SetBorder(true).SetTitle(name)
	panel.SetChangedFunc(func() {
		app.Draw()
	})
	return panel
}

func createOutputPanel(app *tview.Application, infoPanel *tview.Flex, logPanel *tview.TextView) (outputPanel *tview.Flex) {
	outputPanel = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(infoPanel, 10, 1, true).
		AddItem(logPanel, 0, 1, false)
	return outputPanel
}

func createCommandList() (commandList *tview.List) {
	///// Commands /////
	commandList = tview.NewList()
	commandList.SetBorder(true).SetTitle("Commands")
	commandList.ShowSecondaryText(false)
	return commandList
}

func createMainLayout(commandList tview.Primitive, outputPanel tview.Primitive) (layout *tview.Flex) {
	///// Main Layout /////
	mainLayout := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(commandList, 30, 1, true).
		AddItem(outputPanel, 0, 4, false)

	info := tview.NewTextView()
	info.SetBorder(true)
	info.SetText("UDP Packet Tester v1.0 - Copyright 2019 Andrew Young <andrew@vaelen.org>")
	info.SetTextAlign(tview.AlignCenter)

	layout = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(mainLayout, 0, 20, true).
		AddItem(info, 3, 1, false)

	return layout
}

func validFloat(value string, lastChar rune) bool {
	_, err := strconv.ParseFloat(value, 0)
	return err == nil
}

func validInt(value string, lastChar rune) bool {
	_, err := strconv.Atoi(value)
	return err == nil
}

func createModalForm(pages *tview.Pages, form tview.Primitive, height int, width int) tview.Primitive {
	modal := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(form, height, 1, true).
			AddItem(nil, 0, 1, false), width, 1, true).
		AddItem(nil, 0, 1, false)
	return modal
}

func loopbackTestCommand(pages *tview.Pages, infoUI *testInfoUI) func() {
	return func() {
		if infoUI.ctx != nil {
			modal := tview.NewModal().
				SetText("A test is already running.").
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(buttonIndex int, buttonLabel string) {
					pages.SwitchToPage("main")
					pages.RemovePage("modal")
				})
			pages.AddPage("modal", modal, true, true)
			return
		}

		localAddress := ":6001"
		remoteAddress := "127.0.0.1:6000"
		dataRate := 2.0
		packetSize := 223

		startFunc := func() {
			pages.SwitchToPage("main")
			pages.RemovePage("modal")
			ctx, cancel := context.WithCancel(context.Background())
			infoUI.ctx = ctx
			infoUI.cancel = cancel
			go func() {
				defer func() {
					cancel()
					infoUI.ctx = nil
				}()
				loopbackTest(ctx, localAddress, remoteAddress, dataRate, packetSize, infoUI)
			}()
		}

		cancelFunc := func() {
			pages.SwitchToPage("main")
			pages.RemovePage("modal")
		}

		form := tview.NewForm()
		form.AddInputField("Local Address:", localAddress, 30, nil, func(value string) { localAddress = value })
		form.AddInputField("Remote Address:", remoteAddress, 30, nil, func(value string) { remoteAddress = value })
		form.AddInputField("Data Rate (Mbps):", strconv.FormatFloat(dataRate, 'f', -1, 64), 30, validFloat, func(value string) { dataRate, _ = strconv.ParseFloat(value, 64) })
		form.AddInputField("Packet Size (bytes):", strconv.Itoa(packetSize), 30, validInt, func(value string) { packetSize, _ = strconv.Atoi(value) })
		form.AddButton("Start Test", startFunc)
		form.AddButton("Cancel", cancelFunc)
		form.SetCancelFunc(cancelFunc)
		form.SetButtonsAlign(tview.AlignCenter)

		form.SetBorder(true).SetTitle("Loopback Test Options")

		modal := createModalForm(pages, form, 13, 55)

		pages.AddPage("modal", modal, true, true)

	}
}

func listenCommand(pages *tview.Pages, infoUI *testInfoUI) func() {
	return func() {
		if infoUI.ctx != nil {
			modal := tview.NewModal().
				SetText("A test is already running.").
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(buttonIndex int, buttonLabel string) {
					pages.SwitchToPage("main")
					pages.RemovePage("modal")
				})
			pages.AddPage("modal", modal, true, true)
			return
		}

		localAddress := ":6001"

		startFunc := func() {
			pages.SwitchToPage("main")
			pages.RemovePage("modal")
			ctx, cancel := context.WithCancel(context.Background())
			infoUI.ctx = ctx
			infoUI.cancel = cancel
			go func() {
				defer func() {
					cancel()
					infoUI.ctx = nil
				}()
				listen(ctx, localAddress, infoUI)
			}()
		}

		cancelFunc := func() {
			pages.SwitchToPage("main")
			pages.RemovePage("modal")
		}

		form := tview.NewForm()
		form.AddInputField("Local Address:", localAddress, 30, nil, func(value string) { localAddress = value })
		form.AddButton("Start Listening", startFunc)
		form.AddButton("Cancel", cancelFunc)
		form.SetCancelFunc(cancelFunc)
		form.SetButtonsAlign(tview.AlignCenter)

		form.SetBorder(true).SetTitle("Listen Options")

		modal := createModalForm(pages, form, 13, 55)

		pages.AddPage("modal", modal, true, true)

	}
}

func stopTestCommand(infoUI *testInfoUI) func() {
	return func() {
		if infoUI.cancel != nil {
			infoUI.cancel()
		}
	}
}

func loopbackTest(ctx context.Context, localAddress string, remoteAddress string, sendRateInMbps float64, sendDataLength int, ui *testInfoUI) {
	ui.reset()

	ui.running("Loopback Test")
	defer ui.finished("Loopback Test")

	packetsPerSecond := sendRateInMbps / (float64(sendDataLength) * toMbps)
	packetsPerNano := packetsPerSecond / float64((time.Second / time.Nanosecond))
	sendInterval := time.Duration(int64(1.0 / packetsPerNano))

	ui.localAddress(localAddress)
	ui.remoteAddress(remoteAddress)
	ui.interval(sendInterval)
	ui.dataSize(sendDataLength)

	sendData := make([]byte, sendDataLength)
	for i := 0; i < sendDataLength; i++ {
		sendData[i] = 0xff
	}

	client, err := NewDataClient(remoteAddress, localAddress)
	if err != nil {
		log.Printf("Error starting client: %v", err)
		return
	}
	defer client.Close()

	stop := make(chan struct{})
	defer close(stop)
	finished := make(chan struct{})

	go func() {
		defer close(finished)
		log.Printf("Listening: %v", localAddress)
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case r := <-client.Receive():
				dataSize := len(r.Data)
				ui.receivedPacket(dataSize)
				if dataSize != sendDataLength {
					ui.sizeError()
					log.Printf("Received packet with wrong data size. From: %v, Error: %v, Data Size: %v, Data: %X", r.Address, r.Error, dataSize, r.Data)
				}

				if !bytes.Equal(sendData, r.Data) {
					ui.dataError()
					log.Printf("Received packet with wrong data value. From: %v, Error: %v, Data Size: %v, Data: %X", r.Address, r.Error, dataSize, r.Data)
				}

				if r.Error != nil {
					log.Printf("Error receiving, shutting down. Error: %v", r.Error)
					return
				}
			}
		}
	}()

	log.Printf("Sending data. Interval: %v, To: %v, Data Size: %v, Data: %X", sendInterval, remoteAddress, len(sendData), sendData)

	send := time.NewTicker(sendInterval)
	updateCounters := time.NewTicker(time.Millisecond * 100)
	updateRate := time.NewTicker(time.Second)

	ui.updateCounters()
	ui.updateRate()

	for {
		select {
		case <-ctx.Done():
			return
		case <-finished:
			return
		case <-send.C:
			err := client.Send(sendData)
			ui.sentPacket(sendDataLength)
			if err != nil {
				log.Printf("Error sending, shutting down. Error: %v", err)
				return
			}
		case <-updateCounters.C:
			ui.updateCounters()
		case <-updateRate.C:
			ui.updateRate()
		}
	}

}

func listen(ctx context.Context, localAddress string, ui *testInfoUI) {
	ui.reset()

	ui.running("Listening")
	defer ui.finished("Listening")

	ui.localAddress(localAddress)

	client, err := NewDataClient(localAddress, localAddress)
	if err != nil {
		log.Printf("Error starting client: %v", err)
		return
	}
	defer client.Close()

	stop := make(chan struct{})
	defer close(stop)
	finished := make(chan struct{})

	go func() {
		defer close(finished)
		log.Printf("Listening: %v", localAddress)
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case r := <-client.Receive():
				dataSize := len(r.Data)
				ui.receivedPacket(dataSize)
				if r.Error != nil {
					log.Printf("Error receiving, shutting down. Error: %v", r.Error)
					return
				}
			}
		}
	}()

	updateCounters := time.NewTicker(time.Millisecond * 100)
	updateRate := time.NewTicker(time.Second)

	ui.updateCounters()
	ui.updateRate()

	for {
		select {
		case <-ctx.Done():
			return
		case <-finished:
			return
		case <-updateCounters.C:
			ui.updateCounters()
		case <-updateRate.C:
			ui.updateRate()
		}
	}
}

func main() {
	app := createApplication()

	if err := app.Run(); err != nil {
		panic(err)
	}
}
