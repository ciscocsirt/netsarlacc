package main

import (
	"reflect"
	"testing"
)

func TestNewWorker(t *testing.T) {
	type args struct {
		id          int
		workerQueue chan chan WorkRequest
	}
	tests := []struct {
		name string
		args args
		want Worker
	}{
	// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewWorker(tt.args.id, tt.args.workerQueue); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewWorker() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWorker_Start(t *testing.T) {
	type fields struct {
		ID          int
		Work        chan WorkRequest
		WorkerQueue chan chan WorkRequest
		QuitChan    chan bool
	}
	tests := []struct {
		name   string
		fields fields
	}{
	// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &Worker{
				ID:          tt.fields.ID,
				Work:        tt.fields.Work,
				WorkerQueue: tt.fields.WorkerQueue,
				QuitChan:    tt.fields.QuitChan,
			}
			w.Start()
		})
	}
}

func TestWorker_Stop(t *testing.T) {
	type fields struct {
		ID          int
		Work        chan WorkRequest
		WorkerQueue chan chan WorkRequest
		QuitChan    chan bool
	}
	tests := []struct {
		name   string
		fields fields
	}{
	// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &Worker{
				ID:          tt.fields.ID,
				Work:        tt.fields.Work,
				WorkerQueue: tt.fields.WorkerQueue,
				QuitChan:    tt.fields.QuitChan,
			}
			w.Stop()
		})
	}
}

func Test_parseConn(t *testing.T) {
	type args struct {
		buf     []byte
		bufSize int
		req_log *LoggedRequest
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
	// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := parseConn(tt.args.buf, tt.args.bufSize, tt.args.req_log); (err != nil) != tt.wantErr {
				t.Errorf("parseConn() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestToJSON(t *testing.T) {
	type args struct {
		data interface{}
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
	// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToJSON(tt.args.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("ToJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ToJSON() = %v, want %v", got, tt.want)
			}
		})
	}
}
