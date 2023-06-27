package remotecommand

import (
	"bytes"
	"context"
	"io"

	"github.com/gorilla/websocket"
	"k8s.io/klog/v2"
)

func ExecuteConn(ctx context.Context, rawConn *websocket.Conn, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) {
	conn := NewWebsocketConn(rawConn)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// close websocket connection
	defer conn.Close()
	defer func() {
		err := conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		if err != nil {
			klog.Errorf("write close: %v", err)
			return
		}
	}()

	// ping connection
	go func() {
		Ping(ctx, conn)
	}()

	// pipe stdout into websocket
	go func() {
		err := NewStream(conn, StdinData, StdinClose).Read(stdin)
		if err != nil {
			klog.Errorf("pipe stdin: %v", err)
		}
	}()

	// read messages
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return 0, err
		}

		message, err := ParseMessage(bytes.NewReader(raw))
		if err != nil {
			klog.Errorf("Unexpected message: %v", err)
			continue
		}

		if message.messageType == StdoutData {
			if _, err := io.Copy(stdout, message.data); err != nil {
				return 1, err
			}
		} else if message.messageType == StderrData {
			if _, err := io.Copy(stderr, message.data); err != nil {
				return 1, err
			}
		} else if message.messageType == ExitCode {
			return int(message.exitCode), nil
		}
	}
}
