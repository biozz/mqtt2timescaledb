package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/jackc/pgx/v4"
)

var (
	mqttServer = flag.String("mq", "tcp://127.0.0.1:1883", "mqtt server to connect to")
	username   = flag.String("u", "", "username for auth")
	password   = flag.String("p", "", "password for auth")
	dbURL      = flag.String("db", "postgres://postgres:postgres@localhost:5432/measurements", "database to connect to")
	debug      = flag.Bool("vvv", false, "verbose output for debugging")
)

func main() {
	flag.Parse()

	// mqtt stuff
	clientOptions := mqtt.NewClientOptions()
	clientOptions.AddBroker(*mqttServer)
	clientOptions.SetClientID("mqtt2timescaledb")
	clientOptions.SetUsername(*username)
	clientOptions.SetPassword(*password)
	client := mqtt.NewClient(clientOptions)
	token := client.Connect()
	for !token.WaitTimeout(3 * time.Second) {
	}
	if err := token.Error(); err != nil {
		log.Fatalf("Token error: %v", err)
	}
	conn, err := pgx.Connect(context.Background(), *dbURL)
	if err != nil {
		log.Fatalf("Unable to connect to the database: %v", err)
	}
	dcw := DatabaseConnWrapper{DatabaseURL: *dbURL, Conn: conn}
	go func() {
		client.Subscribe("#", 0, dcw.MessageHandler)
	}()

	// wait for exit
	signals := make(chan os.Signal)
	exit := make(chan int)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signals
		exit <- 1
	}()
	log.Println("Waiting for MQTT messages. Press CTRL+C to exit...")
	<-exit
}

type DatabaseConnWrapper struct {
	DatabaseURL string
	Conn        *pgx.Conn
}

func (dcw *DatabaseConnWrapper) MessageHandler(c mqtt.Client, m mqtt.Message) {
	if *debug {
		log.Printf("topic: %s, payload: %s", m.Topic(), string(m.Payload()))
	}

	// I have no idea how to properly handle reconnections
	// but this works well for me.
	err := dcw.Conn.Ping(context.Background())
	if err != nil {
		log.Println("Skip message due to lost db connection")
		dcw.Conn.Close(context.Background())
		conn, err := pgx.Connect(context.Background(), dcw.DatabaseURL)
		if err != nil {
			log.Println("Reconnect attempt failed")
			return
		}

		dcw.Conn = conn
		return
	}

	// actual message handling
	if !strings.Contains(m.Topic(), "/") {
		log.Printf("Metric doesn't follow slash format: %s", m.Topic())
		return
	}
	els := strings.Split(m.Topic(), "/")
	if len(els) != 4 {
		log.Printf("Metric doesn't have enouth elements: %d instead of 4 (%s)", len(els), m.Topic())
		return
	}
	location := els[0]
	room := els[1]
	sensor := els[2]
	measurement := els[3]

	value, err := strconv.ParseFloat(string(m.Payload()), 64)
	if err != nil {
		log.Printf("Unable to convert payload to float: %v", err)
		return
	}

	err = dcw.Conn.BeginFunc(context.Background(), func(tx pgx.Tx) error {
		_, err := tx.Exec(
			context.Background(),
			"INSERT INTO environment (time, location, room, sensor, measurement, value) VALUES (NOW(), $1, $2, $3, $4, $5)",
			location, room, sensor, measurement, value,
		)
		return err
	})
	if err != nil {
		log.Printf("Unable to insert data: %v", err)
		return
	}
}
