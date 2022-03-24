package main

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"fmt"
	"go-charts/internal/metars"
	"go-charts/internal/pireps"
	"go-charts/internal/tafs"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	_ "github.com/mattn/go-sqlite3"
)

func handleHome(w http.ResponseWriter, r *http.Request) {
	setNoCache(w)
	http.FileServer(http.Dir("./html")).ServeHTTP(w, r)
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	scfg, err := GetConfigAsString()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Fprintf(w, scfg)
}

type positionHistory struct {
	ReportTime string  `json:"report_time"`
	Longitude  float64 `json:"longitude"`
	Latitude   float64 `json:"latitude"`
	Heading    int32   `json:"heading"`
	Altitude   int32   `json:"altitude"`
}

type jsonMessage struct {
	MessageType string
	Payload     string
}

func handlePositionHistory(w http.ResponseWriter, r *http.Request) {
	path := "./static/positionhistory.db"
	db, err := sql.Open("sqlite3", "file:"+path+"?mode=ro")
	if err != nil {
		log.Fatal(err)
	}
	sql := "SELECT * FROM position_history WHERE id IN ( SELECT max( id ) FROM position_history )"
	rows, err := db.Query(sql)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var lon, lat float64
		var id, hdg, alt int32
		var dttime string
		rows.Scan(&id, &dttime, &lon, &lat, &hdg, &alt)
		jsonout := fmt.Sprintf(`{ "longitude": %v, "latitude": %v, "heading": %v }`, lon, lat, hdg)
		fmt.Fprintf(w, jsonout)
	}
}

func handleSaveHistory(w http.ResponseWriter, r *http.Request) {
	var ph positionHistory
	d := json.NewDecoder(r.Body)
	err := d.Decode(&ph)
	if err != nil {
		log.Println(err)
	} else {
		path := "./static/positionhistory.db"
		db, err := sql.Open("sqlite3", "file:"+path+"?mode=rw")
		if err != nil {
			log.Println(err)
		}
		sql := fmt.Sprintf("INSERT INTO position_history (datetime, longitude, latitude, heading, gpsaltitude) "+
			"VALUES ('%s', %f, %f, %d, %d)", ph.ReportTime, ph.Longitude, ph.Latitude, ph.Heading, ph.Altitude)

		_, err = db.Exec(sql)
		if err != nil {
			log.Println(err)
		}
	}
	w.WriteHeader(200)
}

func handleAirports(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.RequestURI, "/")
	cid := parts[len(parts)-1]
	w.WriteHeader(200)
	go getAirports(cid)
	return
}

func getAirports(cid string) {
	filehandlerMutex.Lock()
	data, err := os.ReadFile("./static/airports.json") // For read access.
	if err != nil {
		log.Fatal(err)
	}
	filehandlerMutex.Unlock()
	var msg jsonMessage
	msg.MessageType = "airports"
	msg.Payload = string(data)
	sendToClient(msg, cid)
}

func handleDatafiles(w http.ResponseWriter, r *http.Request) {
	filehandlerMutex.Lock()
	parts := strings.Split(r.RequestURI, "/")
	cid := parts[len(parts)-1]
	// read and send METARS json
	data, err := os.ReadFile("./metars.json")
	if err != nil {
		log.Println(err)
	} else {
		var msgmetars jsonMessage
		msgmetars.MessageType = "metars"
		msgmetars.Payload = string(data)
		sendToClient(msgmetars, cid)
	}

	// read and send TAFS json
	data, err = os.ReadFile("./tafs.json")
	if err != nil {
		log.Println(err)
	} else {
		var msgtafs jsonMessage
		msgtafs.MessageType = "tafs"
		msgtafs.Payload = string(data)
		sendToClient(msgtafs, cid)
	}

	// read and send PIREPS json
	data, err = os.ReadFile("./pireps.json")
	if err != nil {
		log.Println(err)
	} else {
		var msgpireps jsonMessage
		msgpireps.MessageType = "pireps"
		msgpireps.Payload = string(data)
		sendToClient(msgpireps, cid)
	}
	filehandlerMutex.Unlock()
}

type mbTileConnectionCacheEntry struct {
	Path     string
	Conn     *sql.DB
	Metadata map[string]string
	fileTime time.Time
}

func (mbtc *mbTileConnectionCacheEntry) IsOutdated() bool {
	file, err := os.Stat(mbtc.Path)
	if err != nil {
		return true
	}
	modTime := file.ModTime()
	return modTime != mbtc.fileTime
}

func newMbTileConnectionCacheEntry(path string, conn *sql.DB) *mbTileConnectionCacheEntry {
	file, err := os.Stat(path)
	if err != nil {
		return nil
	}
	return &mbTileConnectionCacheEntry{path, conn, nil, file.ModTime()}
}

var mbtileCacheLock = sync.Mutex{}
var mbtileConnectionCache = make(map[string]mbTileConnectionCacheEntry)

func connectMbTilesArchive(path string) (*sql.DB, map[string]string, error) {
	mbtileCacheLock.Lock()
	defer mbtileCacheLock.Unlock()
	if conn, ok := mbtileConnectionCache[path]; ok {
		if !conn.IsOutdated() {
			return conn.Conn, conn.Metadata, nil
		}
		log.Printf("Reloading MBTiles " + path)
	}

	conn, err := sql.Open("sqlite3", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, nil, err
	}
	cacheEntry := newMbTileConnectionCacheEntry(path, conn)
	cacheEntry.Metadata = readMbTilesMetadata(path, conn)
	if cacheEntry != nil {
		mbtileConnectionCache[path] = *cacheEntry
	}
	return conn, cacheEntry.Metadata, nil
}

func tileToDegree(z, x, y int) (lon, lat float64) {
	// osm-like schema:
	y = (1 << z) - y - 1
	n := math.Pi - 2.0*math.Pi*float64(y)/math.Exp2(float64(z))
	lat = 180.0 / math.Pi * math.Atan(0.5*(math.Exp(n)-math.Exp(-n)))
	lon = float64(x)/math.Exp2(float64(z))*360.0 - 180.0
	return lon, lat
}

func readMbTilesMetadata(fname string, db *sql.DB) map[string]string {
	rows, err := db.Query(`SELECT name, value FROM metadata 
		UNION SELECT 'minzoom', min(zoom_level) FROM tiles WHERE NOT EXISTS (SELECT * FROM metadata WHERE name='minzoom' and value is not null and value != '')
		UNION SELECT 'maxzoom', max(zoom_level) FROM tiles WHERE NOT EXISTS (SELECT * FROM metadata WHERE name='maxzoom' and value is not null and value != '')`)
	if err != nil {
		log.Printf("SQLite read error %s: %s", fname, err.Error())
		return nil
	}
	defer rows.Close()
	meta := make(map[string]string)
	for rows.Next() {
		var name, val string
		rows.Scan(&name, &val)
		if len(val) > 0 {
			meta[name] = val
		}
	}
	// determine extent of layer if not given.. Openlayers kinda needs this, or it can happen that it tries to do
	// a billion request do down-scale high-res pngs that aren't even there (i.e. all 404s)
	if _, ok := meta["bounds"]; !ok {
		maxZoomInt, _ := strconv.ParseInt(meta["maxzoom"], 10, 32)
		rows, err = db.Query("SELECT min(tile_column), min(tile_row), max(tile_column), max(tile_row) FROM tiles WHERE zoom_level=?", maxZoomInt)
		if err != nil {
			log.Printf("SQLite read error %s: %s", fname, err.Error())
			return nil
		}
		rows.Next()
		var xmin, ymin, xmax, ymax int
		rows.Scan(&xmin, &ymin, &xmax, &ymax)
		lonmin, latmin := tileToDegree(int(maxZoomInt), xmin, ymin)
		lonmax, latmax := tileToDegree(int(maxZoomInt), xmax+1, ymax+1)
		meta["bounds"] = fmt.Sprintf("%f,%f,%f,%f", lonmin, latmin, lonmax, latmax)
	}

	// check if it is vectortiles and we have a style, then add the URL to metadata...
	if format, ok := meta["format"]; ok && format == "pbf" {
		_, file := filepath.Split(fname)
		if _, err := os.Stat("./static/data/styles/" + file + "/style.json"); err == nil {
			// We found a style!
			meta["stratux_style_url"] = "./static/data/styles/" + file + "/style.json"
		}

	}
	return meta
}

// Scans data dir for all .db and .mbtiles files and returns json representation of all metadata values
func handleTilesets(w http.ResponseWriter, r *http.Request) {
	files, err := ioutil.ReadDir("./static/data/")
	if err != nil {
		log.Printf("handleTilesets() error: %s\n", err.Error())
		http.Error(w, err.Error(), 500)
	}
	result := make(map[string]map[string]string, 0)
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if strings.HasSuffix(f.Name(), ".mbtiles") || strings.HasSuffix(f.Name(), ".db") {
			_, meta, err := connectMbTilesArchive("./static/data/" + f.Name())
			if err != nil {
				log.Printf("SQLite open "+f.Name()+" failed: %s", err.Error())
				continue
			}
			result[f.Name()] = meta
		}
	}
	resJSON, _ := json.Marshal(result)
	w.Write(resJSON)
}

func loadTile(fname string, z, x, y int) ([]byte, error) {
	db, meta, err := connectMbTilesArchive("./static/data/" + fname)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query("SELECT tile_data FROM tiles WHERE zoom_level=? AND tile_column=? AND tile_row=?", z, x, y)
	if err != nil {
		log.Printf("Failed to query mbtiles: %s", err.Error())
		return nil, nil
	}

	defer rows.Close()
	for rows.Next() {
		var res []byte
		rows.Scan(&res)
		// sometimes pbfs are gzipped...
		if format, ok := meta["format"]; ok && format == "pbf" && len(res) >= 2 && res[0] == 0x1f && res[1] == 0x8b {
			reader := bytes.NewReader(res)
			gzreader, _ := gzip.NewReader(reader)
			unzipped, err := ioutil.ReadAll(gzreader)
			if err != nil {
				log.Printf("Failed to unzip gzipped PBF data")
				return nil, nil
			}
			res = unzipped
		}
		return res, nil
	}
	return nil, nil
}

func handleTile(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.RequestURI, "/")
	if len(parts) < 4 {
		return
	}
	idx := len(parts) - 1
	y, err := strconv.Atoi(strings.Split(parts[idx], ".")[0])
	if err != nil {
		http.Error(w, "Failed to parse y", 500)
		return
	}
	idx--
	x, _ := strconv.Atoi(parts[idx])
	idx--
	z, _ := strconv.Atoi(parts[idx])
	idx--
	file, _ := url.QueryUnescape(parts[idx])

	tileData, err := loadTile(file, z, x, y)
	if err != nil {
		http.Error(w, err.Error(), 500)
	} else if tileData == nil {
		http.Error(w, "Tile not found", 404)
	} else {
		w.Write(tileData)
	}
}

func setNoCache(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

func setJSONHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
}

var filehandlerMutex = sync.Mutex{}

func downloadDataFiles() {
	filehandlerMutex.Lock()

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		var mr metars.Response
		err := mr.SaveAsJSONFile(config.MetarsURL)
		if err != nil {
			log.Println("Error downloading Metars file", err)
		}
		wg.Done()
	}()

	go func() {
		var tr tafs.Response
		err := tr.SaveAsJSONFile(config.TafsURL)
		if err != nil {
			log.Println("Error downloading Tafs file", err)
		}
		wg.Done()
	}()

	go func() {
		var pr pireps.Response
		err := pr.SaveAsJSONFile(config.PirepsURL)
		if err != nil {
			log.Println("Error downloading Pireps file", err)
		}
		wg.Done()
	}()
	wg.Wait()

	filehandlerMutex.Unlock()
}

func timedDataFileDownload() {
	ticker := time.NewTicker(6 * time.Minute)
	for range ticker.C {
		downloadDataFiles()
	}
}

func sendToClient(message jsonMessage, cid string) {
	for client := range clients {
		if client.ID == cid {
			log.Printf("Sending data to client %s", cid)
			err := client.WriteJSON(message)
			if err != nil {
				// the user left the page, or their connection dropped
				log.Println(err)
				_ = client.Close()
				delete(clients, client)
			}
		}
	}
}

var upgradeConnection = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
}

type webSocketConnection struct {
	*websocket.Conn
	ID string
}

var clients = make(map[webSocketConnection]string)

func handleWsEndpoint(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.RequestURI, "/")
	cid := parts[len(parts)-1]
	ws, err := upgradeConnection.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
	}
	conn := webSocketConnection{Conn: ws, ID: cid}
	clients[conn] = ""
	log.Printf("Websocket connected to client %s", cid)
	go listenForWs(&conn)
}

func listenForWs(conn *webSocketConnection) {
	defer func() {
		if r := recover(); r != nil {
			log.Println("Error in ListenForWs defer func()")
			log.Println("Error", fmt.Sprintf("%v", r))
		}
	}()
	go readLoop(conn)
}

func readLoop(conn *webSocketConnection) {
	for {
		if _, r, err := conn.NextReader(); err != nil {
			conn.Close()
			delete(clients, *conn)
			log.Println("Client closed endpoint")
			break
		} else {
			// client heartbeat is a timestamp, just ignore it
			_, err := ioutil.ReadAll(r)
			if err != nil {
				log.Println(err)
			}
		}
	}
}

func main() {
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/ws/", handleWsEndpoint)
	http.HandleFunc("/getconfig", handleConfig)
	http.HandleFunc("/gethistory", handlePositionHistory)
	http.HandleFunc("/tiles/tilesets", handleTilesets)
	http.HandleFunc("/tiles/", handleTile)
	http.HandleFunc("/getdatafiles/", handleDatafiles)
	http.HandleFunc("/getairports/", handleAirports)
	http.HandleFunc("/savehistory", handleSaveHistory)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static/"))))

	err := LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	downloadDataFiles()
	go timedDataFileDownload()

	addr := fmt.Sprintf(":%d", config.Httpport)
	log.Printf("Starting web server on port %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
