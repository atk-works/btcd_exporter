package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const namespace = "btcd"

var (
	up = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "up"),
		"Was the last btcd query successful.",
		nil, nil,
	)
	blocks = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "blocks_total"),
		"How many blocks are reported by btcd getinfo.",
		nil, nil,
	)
	latestBlock = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "latest_block_timestamp"),
		"Timestamp of the latest block in the chain. According to block header information.",
		nil, nil,
	)
	peers = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "peers"),
		"How many peers are reported by btcd getinfo.",
		nil, nil,
	)
	difficulty = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "difficulty"),
		"What is difficulty reported by btcd getinfo.",
		nil, nil,
	)
	bytesSent = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "sent_bytes"),
		"How many bytes have been sent reported by btcd getnettotals.",
		nil, nil,
	)
	bytesReceived = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "received_bytes"),
		"How many bytes have been received reported by btcd getnettotals.",
		nil, nil,
	)
)

type BtcdStatistics struct {
	blocks        int
	peers         int
	difficulty    float64
	bytesSent     int
	bytesReceived int
	latestBlockTs int
}

func newBtcdStatistics(blocks int, peers int, difficulty float64, bytesSent int, bytesReceived int, latestBlockTs int) *BtcdStatistics {
	return &BtcdStatistics{
		blocks:        blocks,
		peers:         peers,
		difficulty:    difficulty,
		bytesSent:     bytesSent,
		bytesReceived: bytesReceived,
		latestBlockTs: latestBlockTs,
	}
}

type Exporter struct {
	client *rpcclient.Client
}

func NewExporter(client *rpcclient.Client) *Exporter {
	return &Exporter{
		client: client,
	}
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- up
	ch <- blocks
	ch <- peers
	ch <- difficulty
	ch <- bytesSent
	ch <- bytesReceived
	ch <- latestBlock
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	statistics, err := e.GetAllStatistics()
	if err != nil {
		ch <- prometheus.MustNewConstMetric(
			up, prometheus.GaugeValue, 0,
		)
		log.Println(err)
		return
	}
	ch <- prometheus.MustNewConstMetric(
		up, prometheus.GaugeValue, 1,
	)
	ch <- prometheus.MustNewConstMetric(blocks, prometheus.CounterValue, float64(statistics.blocks))
	ch <- prometheus.MustNewConstMetric(peers, prometheus.GaugeValue, float64(statistics.peers))
	ch <- prometheus.MustNewConstMetric(difficulty, prometheus.GaugeValue, statistics.difficulty)
	ch <- prometheus.MustNewConstMetric(bytesSent, prometheus.CounterValue, float64(statistics.bytesSent))
	ch <- prometheus.MustNewConstMetric(bytesReceived, prometheus.GaugeValue, float64(statistics.bytesReceived))
	ch <- prometheus.MustNewConstMetric(latestBlock, prometheus.GaugeValue, float64(statistics.latestBlockTs))
}

func (e *Exporter) GetAllStatistics() (*BtcdStatistics, error) {
	info, err := e.client.GetInfo()
	if err != nil {
		return nil, err
	}
	netTotals, err := e.client.GetNetTotals()
	if err != nil {
		return nil, err
	}
	bestBlockHash, err := e.client.GetBestBlockHash()
	if err != nil {
		return nil, err
	}
	blockHeader, err := e.client.GetBlockHeader(bestBlockHash)
	if err != nil {
		return nil, err
	}
	statistics := newBtcdStatistics(
		int(info.Blocks),
		int(info.Connections),
		info.Difficulty,
		int(netTotals.TotalBytesSent),
		int(netTotals.TotalBytesRecv),
		int(blockHeader.Timestamp.Unix()),
	)
	return statistics, nil
}

func main() {
	host := os.Getenv("BTCD_EXPORTER_HOST")
	username := os.Getenv("BTCD_EXPORTER_USERNAME")
	password := os.Getenv("BTCD_EXPORTER_PASSWORD")
	certPath := os.Getenv("BTCD_EXPORTER_CERT_PATH")
	if host == "" || username == "" || password == "" {
		log.Fatal("BTCD_EXPORTER_HOST, BTCD_EXPORTER_USERNAME, BTCD_EXPORTER_PASSWORD must be set")
	}
	if certPath == "" {
		btcdHomeDir := btcutil.AppDataDir("btcd", false)
		certPath = filepath.Join(btcdHomeDir, "rpc.cert")
		log.Println("BTCD_EXPORTER_CERT_PATH not set, using default path: ", certPath)
	}
	certs, err := ioutil.ReadFile(certPath)
	if err != nil {
		log.Fatal("error reading cert file: ", err)
	}
	connCfg := &rpcclient.ConnConfig{
		Host:         host,
		Endpoint:     "ws",
		User:         username,
		Pass:         password,
		Certificates: certs,
	}
	client, err := rpcclient.New(connCfg, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Shutdown()

	exporter := NewExporter(client)
	prometheus.MustRegister(exporter)
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
             <head><title>BTCD Exporter</title></head>
             <body>
             <h1>BTCD Exporterr</h1>
             <p><a href='/metrics'>Metrics</a></p>
             </body>
             </html>`))
	})
	log.Println("starting server on port 9101")
	log.Fatal(http.ListenAndServe(":9101", nil))
}
