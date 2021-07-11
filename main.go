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
	dbServer   = flag.String("db", "postgres://postgres:postgres@localhost:5432/measurements", "database to connect to")
	debug      = flag.Bool("vvv", false, "verbose output for debugging")
)

func main() {
	flag.Parse()
	// db stuff
	conn, err := pgx.Connect(context.Background(), *dbServer)
	if err != nil {
		log.Fatalf("Unable to connect to the database: %v", err)
	}
	defer conn.Close(context.Background())

	// mqtt stuff
	clientOptions := mqtt.NewClientOptions()
	clientOptions.AddBroker(*mqttServer)
	clientOptions.SetClientID("mqtt2timescaledb")
	client := mqtt.NewClient(clientOptions)
	token := client.Connect()
	for !token.WaitTimeout(3 * time.Second) {
	}
	if err := token.Error(); err != nil {
		log.Fatalf("Token error: %v", err)
	}
	mc := MeasurementConverter{Conn: conn}
	go func() {
		client.Subscribe("#", 0, mc.MessageHandler)
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

type MeasurementConverter struct {
	Conn *pgx.Conn
}

func (mc *MeasurementConverter) MessageHandler(c mqtt.Client, m mqtt.Message) {
	if *debug {
		log.Printf("topic: %s, payload: %s", m.Topic(), string(m.Payload()))
	}
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

	err = mc.Conn.BeginFunc(context.Background(), func(tx pgx.Tx) error {
		_, err := tx.Exec(
			context.Background(),
			"INSERT INTO environment (room, location, sensor, measurement, value) VALUES ($1, $2, $3, $4, $5)",
			location, room, sensor, measurement, value,
		)
		return err
	})
	if err != nil {
		log.Printf("Unable to insert data: %v", err)
		return
	}
}
