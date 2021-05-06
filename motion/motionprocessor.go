// thermal-recorder - record thermal video footage of warm moving objects
//  Copyright (C) 2018, The Cacophony Project
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <http://www.gnu.org/licenses/>.

package motion

import (
	"errors"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/TheCacophonyProject/go-cptv/cptvframe"
	"github.com/TheCacophonyProject/window"

	config "github.com/TheCacophonyProject/go-config"
	"github.com/TheCacophonyProject/thermal-recorder/loglimiter"
	"github.com/TheCacophonyProject/thermal-recorder/recorder"
)

const minLogInterval = time.Minute

type FrameParser func([]byte, *cptvframe.Frame, int) error

func NewMotionProcessor(
	parseFrame FrameParser,
	motionConf *config.ThermalMotion,
	recorderConf *recorder.RecorderConfig,
	locationConf *config.Location,
	listener RecordingListener,
	recorder recorder.Recorder, c cptvframe.CameraSpec,
) *MotionProcessor {

	makeActiveArea()
	printActiveArea()

	return &MotionProcessor{
		parseFrame:     parseFrame,
		minFrames:      recorderConf.MinSecs * c.FPS(),
		maxFrames:      recorderConf.MaxSecs * c.FPS(),
		motionDetector: NewMotionDetector(*motionConf, recorderConf.PreviewSecs*c.FPS(), c),
		frameLoop:      NewFrameLoop(recorderConf.PreviewSecs*c.FPS()+motionConf.TriggerFrames, c),
		isRecording:    false,
		window:         recorderConf.Window,
		listener:       listener,
		conf:           recorderConf,
		triggerFrames:  motionConf.TriggerFrames,
		recorder:       recorder,
		locationConfig: locationConf,
		log:            loglimiter.New(minLogInterval),
	}
}

var activeArea [24][32]bool

type Point struct {
	x float32
	y float32
}

var p1 = Point{
	x: 5,
	y: 5,
}

var p2 = Point{
	x: 24,
	y: 5,
}

var p3 = Point{
	x: 8,
	y: 20,
}

var p4 = Point{
	x: 21,
	y: 20,
}

func makeActiveArea() {
	for y, row := range activeArea {
		for x := range row {
			activeArea[y][x] = pointInTriangle2(p1, p2, p3, float32(x), float32(y)) || pointInTriangle2(p2, p3, p4, float32(x), float32(y))
		}
	}
}

func printActiveArea() {
	area := ""
	for y, row := range activeArea {
		line := ""
		for _, val := range row {
			if val {
				line = line + "###"
			} else {
				line = line + "___"
			}
		}
		area = fmt.Sprintf("%v\n%v: %v", area, line, y)

	}
	log.Println(area)
}

func pointInTriangle2(p1, p2, p3 Point, x, y float32) bool {
	a := ((p2.y-p3.y)*(x-p3.x) + (p3.x-p2.x)*(y-p3.y)) / ((p2.y-p3.y)*(p1.x-p3.x) + (p3.x-p2.x)*(p1.y-p3.y))
	b := ((p3.y-p1.y)*(x-p3.x) + (p1.x-p3.x)*(y-p3.y)) / ((p2.y-p3.y)*(p1.x-p3.x) + (p3.x-p2.x)*(p1.y-p3.y))
	c := 1 - a - b
	return 0 <= a && a <= 1 && 0 <= b && b <= 1 && 0 <= c && c <= 1
}

type MotionProcessor struct {
	parseFrame          FrameParser
	minFrames           int
	maxFrames           int
	framesWritten       int
	motionDetector      *motionDetector
	frameLoop           *FrameLoop
	isRecording         bool
	writeUntil          int
	window              window.Window
	conf                *recorder.RecorderConfig
	listener            RecordingListener
	triggerFrames       int
	triggered           int
	recorder            recorder.Recorder
	locationConfig      *config.Location
	sunriseSunsetWindow bool
	sunriseOffset       int
	sunsetOffset        int
	nextSunriseCheck    time.Time
	log                 *loglimiter.LogLimiter
}

type RecordingListener interface {
	MotionDetected()
	RecordingStarted()
	RecordingEnded()
}

func (mp *MotionProcessor) Reset(camera cptvframe.CameraSpec) {
	mp.stopRecording()
	mp.motionDetector.Reset(camera)
}

func (mp *MotionProcessor) Process(rawFrame []byte) error {
	frame := mp.frameLoop.Current()
	if err := mp.parseFrame(rawFrame, frame, mp.motionDetector.start); err != nil {
		mp.stopRecording()
		return err
	}
	mp.process(frame)
	return nil
}

const tempThresh = 20000

func mlxDetect(frame *cptvframe.Frame) bool {
	var max uint16 = 0
	var min uint16 = math.MaxUint16
	for y, row := range frame.Pix {
		for x, val := range row {
			if activeArea[y][x] {
				max = uint16Max(max, val)
				min = uint16Min(min, val)
			}
		}
	}
	if max-min > 4000 {
		//log.Printf("Maximum %v", max)
		//log.Printf("Minimum %v", min)
		//log.Printf("Diff: %v", max-min)
		return true
	}
	return false
}

func uint16Max(a, b uint16) uint16 {
	if a > b {
		return a
	}
	return b
}

func uint16Min(a, b uint16) uint16 {
	if a < b {
		return a
	}
	return b
}

func (mp *MotionProcessor) process(frame *cptvframe.Frame) {
	if mp.motionDetector.Detect(frame) {
		if mp.listener != nil {
			mp.listener.MotionDetected()
		}
		mp.triggered++

		if mp.isRecording {
			// increase the length of recording
			mp.writeUntil = min(mp.framesWritten+mp.minFrames, mp.maxFrames)
		} else if mp.triggered < mp.triggerFrames {
			// Only start recording after n (triggerFrames) consecutive frames with motion detected.
		} else if err := mp.canStartWriting(); err != nil {
			mp.log.Printf("Recording not started: %v", err)
		} else if err := mp.startRecording(); err != nil {
			mp.log.Printf("Can't start recording file: %v", err)
		} else {
			mp.writeUntil = mp.minFrames
		}
	} else {
		mp.triggered = 0
	}

	// If recording, write the frame.
	if mp.isRecording {
		err := mp.recorder.WriteFrame(frame)
		if err != nil {
			mp.log.Printf("Failed to write to CPTV file %v", err)
		}
		mp.framesWritten++
	}

	mp.frameLoop.Move()

	if mp.isRecording && mp.framesWritten >= mp.writeUntil {
		err := mp.stopRecording()
		if err != nil {
			mp.log.Printf("Failed to stop recording CPTV file %v", err)
		}
	}
}

func (mp *MotionProcessor) ProcessFrame(srcFrame *cptvframe.Frame) {
	frame := mp.frameLoop.Current()
	frame.Copy(srcFrame)
	mp.process(frame)
}

func (mp *MotionProcessor) GetRecentFrame() *cptvframe.Frame {
	return mp.frameLoop.CopyRecent()
}

func (mp *MotionProcessor) canStartWriting() error {
	if !mp.window.Active() {
		return errors.New("motion detected but outside of recording window")
	}
	return mp.recorder.CheckCanRecord()
}

func (mp *MotionProcessor) startRecording() error {

	if err := mp.recorder.StartRecording(mp.motionDetector.background, mp.motionDetector.tempThresh); err != nil {
		return err
	}

	mp.isRecording = true
	if mp.listener != nil {
		mp.listener.RecordingStarted()
	}

	return mp.recordPreTriggerFrames()
}

func (mp *MotionProcessor) stopRecording() error {
	if !mp.isRecording {
		return nil
	}
	if mp.listener != nil {
		mp.listener.RecordingEnded()
	}

	err := mp.recorder.StopRecording()

	mp.framesWritten = 0
	mp.writeUntil = 0
	mp.isRecording = false
	mp.triggered = 0
	// if it starts recording again very quickly it won't write the same frames again
	mp.frameLoop.SetAsOldest()

	return err
}

func (mp *MotionProcessor) recordPreTriggerFrames() error {
	frames := mp.frameLoop.GetHistory()
	var frame *cptvframe.Frame
	ii := 0

	// it never writes the current frame as this will be written later
	for ii < len(frames)-1 {
		frame = frames[ii]
		if err := mp.recorder.WriteFrame(frame); err != nil {
			return err
		}
		ii++
	}

	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
