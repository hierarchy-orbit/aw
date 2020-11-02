package channel_test

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/renproject/aw/channel"
	"github.com/renproject/aw/codec"
	"github.com/renproject/aw/handshake"
	"github.com/renproject/aw/policy"
	"github.com/renproject/aw/tcp"
	"github.com/renproject/aw/wire"
	"github.com/renproject/id"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Channels", func() {

	run := func(ctx context.Context, self id.Signatory) (inbound <-chan wire.Msg, outbound chan<- wire.Msg) {
		inbound, outbound := make(chan wire.Msg), make(chan wire.Msg)
		ch := channel.New(self, inbound, outbound)
		go func() {
			defer GinkgoRecover()
			if err := ch.Run(ctx); err != nil {
				log.Printf("run: %v", err)
				return
			}
		}()
	}

	sink := func(outbound chan<- wire.Msg, n uint64) <-chan struct{} {
		quit := make(chan struct{})
		go func() {
			defer close(quit)
			for iter := uint64(0); iter < n; iter++ {
				data := [8]byte{}
				binary.BigEndian.PutUint64(data[:], iter)
				outbound <- wire.Msg{Data: data}
			}
		}()
		return quit
	}

	stream := func(inbound <-chan wire.Msg, n uint64) <-chan struct{} {
		quit := make(chan struct{})
		go func() {
			defer close(quit)
			for iter := uint64(0); iter < n; iter++ {
				msg := <-inbound
				data := binary.BigEndian.Uint64(msg.Data)
				Expect(data).To(Equal(iter))
			}
		}()
		return quit
	}

	listen := func(ctx context.Context, ch *channel.Channel, self, other id.Signatory, port uint16) {
		go func() {
			defer GinkgoRecover()
			Expect(tcp.Listen(
				ctx,
				fmt.Sprintf("127.0.0.1:%v", port),
				func(conn net.Conn) {
					log.Printf("accepted: %v", conn.RemoteAddr())
					enc, dec, remote, err := handshake.Insecure(self)(
						conn,
						codec.LengthPrefixEncoder(codec.PlainEncoder),
						codec.LengthPrefixDecoder(codec.PlainDecoder),
					)
					if err != nil {
						log.Printf("handshake: %v", err)
						return
					}
					if !other.Equal(&remote) {
						log.Printf("handshake: expected %v, got %v", other, remote)
						return
					}
					if err := ch.Attach(ctx, conn, enc, dec); err != nil {
						log.Printf("attach listener: %v", err)
						return
					}
				},
				func(err error) {
					log.Printf("listen: %v", err)
				},
				nil,
			)).To(Equal(context.Canceled))
		}()
	}

	dial := func(ctx context.Context, ch *channel.Channel, self, other id.Signatory, port uint64, retry time.Duration) {
		go func() {
			defer GinkgoRecover()
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				// Dial a connection in the background. We do it in the
				// background so that we can dial new connections later (to
				// replace this one, and verify that channels behave as
				// expected under these conditions).
				go func() {
					defer GinkgoRecover()
					Expect(tcp.Dial(
						ctx,
						fmt.Sprintf("127.0.0.1:%v", port),
						func(conn net.Conn) {
							log.Printf("dialed: %v", conn.RemoteAddr())
							enc, dec, remote, err := handshake.Insecure(self)(
								conn,
								codec.LengthPrefixEncoder(codec.PlainEncoder),
								codec.LengthPrefixDecoder(codec.PlainDecoder),
							)
							if err != nil {
								log.Printf("handshake: %v", err)
								return
							}
							if !other.Equal(&remote) {
								log.Printf("handshake: expected %v, got %v", other, remote)
								return
							}
							if err := ch.Attach(ctx, conn, enc, dec); err != nil {
								log.Printf("attach dialer: %v", err)
								return
							}
						},
						func(err error) {
							log.Printf("dial: %v", err)
						},
						policy.ConstantTimeout(100*time.Millisecond),
					)).To(Succeed())
				}()
				// After some duration, dial again. This will create an
				// entirely new connection, and replace the previous
				// connection.
				<-time.After(retry)
			}
		}()
	}

	Context("when a connection is attached before sending messages", func() {
		It("should send and receive all message in order", func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Run the local channel.
			localPrivKey := id.NewPrivKey()
			localInbound, localOutbound := run(ctx, localPrivKey.Signatory())
			// Run the remote channel.
			remotePrivKey := id.NewPrivKey()
			remoteInbound, remoteOutbound := run(ctx, remotePrivKey.Signatory())

			// Remote channel will listen for incoming connections.
			listen(ctx, ch, remotePrivKey.Signatory(), localPrivKey.Signatory(), 3333)
			// Local channel will dial the listener (and re-dial once per
			// minute; so it should not impact the test, which is expected
			// to complete in less than one minute).
			dial(ctx, ch, localPrivKey.Signatory(), remotePrivKey.Signatory(), 3333, time.Minute)

			// Wait for the connections to be attached before beginning to
			// send/receive messages.
			time.Sleep(time.Second)

			// Number of messages that we will test.
			n := uint64(1000)
			// Send and receive messages in both direction; from local to
			// remote, and from remote to local.
			q1 := sink(localOutbound, n)
			q2 := stream(remoteInbound, n)
			q3 := sink(remoteOutbound, n)
			q4 := stream(localInbound, n)

			<-q1
			<-q2
			<-q3
			<-q4
		})
	})

	Context("when a connection is attached after sending messages", func() {
		It("should send and receive all message in order", func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Run the local channel.
			localPrivKey := id.NewPrivKey()
			localInbound, localOutbound := run(ctx, localPrivKey.Signatory())
			// Run the remote channel.
			remotePrivKey := id.NewPrivKey()
			remoteInbound, remoteOutbound := run(ctx, remotePrivKey.Signatory())

			// Number of messages that we will test.
			n := uint64(1000)
			// Send and receive messages in both direction; from local to
			// remote, and from remote to local.
			q1 := sink(localOutbound, n)
			q2 := stream(remoteInbound, n)
			q3 := sink(remoteOutbound, n)
			q4 := stream(localInbound, n)

			// Wait for some messages to begin being sent/received before
			// attaching network connections.
			time.Sleep(time.Second)

			// Remote channel will listen for incoming connections.
			listen(ctx, ch, remotePrivKey.Signatory(), localPrivKey.Signatory(), 3334)
			// Local channel will dial the listener (and re-dial once per
			// minute; so it should not impact the test, which is expected
			// to complete in less than one minute).
			dial(ctx, ch, localPrivKey.Signatory(), remotePrivKey.Signatory(), 3334, time.Minute)

			<-q1
			<-q2
			<-q3
			<-q4
		})
	})

	Context("when a connection is replaced while sending messages", func() {
		It("should send and receive all messages in order", func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Run the local channel.
			localPrivKey := id.NewPrivKey()
			localInbound, localOutbound := run(ctx, localPrivKey.Signatory())
			// Run the remote channel.
			remotePrivKey := id.NewPrivKey()
			remoteInbound, remoteOutbound := run(ctx, remotePrivKey.Signatory())

			// Number of messages that we will test.
			n := uint64(1000)
			// Send and receive messages in both direction; from local to
			// remote, and from remote to local.
			q1 := sink(localOutbound, n)
			q2 := stream(remoteInbound, n)
			q3 := sink(remoteOutbound, n)
			q4 := stream(localInbound, n)

			// Remote channel will listen for incoming connections.
			listen(ctx, ch, remotePrivKey.Signatory(), localPrivKey.Signatory(), 3335)
			// Local channel will dial the listener (and re-dial once per
			// second).
			dial(ctx, ch, localPrivKey.Signatory(), remotePrivKey.Signatory(), 3335, time.Second)

			// Wait for sinking and streaming to finish.
			<-q1
			<-q2
			<-q3
			<-q4
		})
	})
})