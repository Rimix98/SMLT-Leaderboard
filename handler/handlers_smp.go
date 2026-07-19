package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

type SMPStatus struct {
	Online       bool   `json:"online"`
	PlayersMax   int    `json:"playersMax"`
	PlayersOnline int   `json:"playersOnline"`
	Version      string `json:"version"`
	ServerIP     string `json:"serverIp"`
	FetchedAt    string `json:"fetchedAt"`
}

const (
	smpServerAddr = "94.154.11.166:25565"
	smpTimeout   = 3 * time.Second
)

func handleSMPStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	cacheKey := "smp-status"
	if cached, ok := cacheGet(cacheKey); ok {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=30, stale-while-revalidate=60")
		w.Header().Set("X-Cache", "HIT")
		w.Write(cached)
		return
	}

	status := pingMinecraftServer(smpServerAddr)

	body, _ := json.Marshal(status)
	cacheSet(cacheKey, body, 30*time.Second)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=30, stale-while-revalidate=60")
	w.Header().Set("X-Cache", "MISS")
	w.Write(body)
}

func pingMinecraftServer(addr string) SMPStatus {
	status := SMPStatus{
		Online:   false,
		ServerIP: "94.154.11.166",
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	}

	conn, err := net.DialTimeout("tcp", addr, smpTimeout)
	if err != nil {
		slog.Warn("SMP server ping failed (TCP)", "addr", addr, "error", err)
		return status
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(smpTimeout))

	// Minecraft server list ping: https://wiki.vg/Server_List_Ping
	// Handshake packet
	host := "94.154.11.166"
	port := 25565
	handshake := buildHandshakePacket(host, port)
	if _, err := conn.Write(handshake); err != nil {
		slog.Warn("SMP handshake write failed", "error", err)
		return status
	}

	// Status request
	if _, err := conn.Write([]byte{0x01, 0x00}); err != nil {
		slog.Warn("SMP status request write failed", "error", err)
		return status
	}

	// Read response length
	length, err := readVarInt(conn)
	if err != nil || length <= 0 {
		slog.Warn("SMP response length read failed", "error", err)
		return status
	}

	// Read packet ID
	packetID, err := readVarInt(conn)
	if err != nil || packetID != 0x00 {
		slog.Warn("SMP unexpected packet ID", "id", packetID, "error", err)
		return status
	}

	// Read JSON length
	jsonLen, err := readVarInt(conn)
	if err != nil || jsonLen <= 0 {
		slog.Warn("SMP JSON length read failed", "error", err)
		return status
	}

	// Read JSON
	jsonBuf := make([]byte, jsonLen)
	if _, err := readFull(conn, jsonBuf); err != nil {
		slog.Warn("SMP JSON read failed", "error", err)
		return status
	}

	var resp struct {
		Version struct {
			Name     string `json:"name"`
			Protocol int    `json:"protocol"`
		} `json:"version"`
		Players struct {
			Max    int `json:"max"`
			Online int `json:"online"`
		} `json:"players"`
	}
	if err := json.Unmarshal(jsonBuf, &resp); err != nil {
		slog.Warn("SMP JSON unmarshal failed", "error", err)
		return status
	}

	status.Online = true
	status.PlayersMax = resp.Players.Max
	status.PlayersOnline = resp.Players.Online
	status.Version = resp.Version.Name

	return status
}

func buildHandshakePacket(host string, port int) []byte {
	var buf []byte
	buf = appendVarInt(buf, 0x00) // packet ID
	buf = appendVarInt(buf, 760)  // protocol version (1.19.4)
	buf = appendVarInt(buf, len(host))
	buf = append(buf, host...)
	buf = append(buf, byte(port>>8), byte(port))
	buf = appendVarInt(buf, 1) // next state: status
	return prependVarInt(buf)
}

func appendVarInt(buf []byte, value int) []byte {
	for {
		b := byte(value & 0x7F)
		value >>= 7
		if value != 0 {
			b |= 0x80
		}
		buf = append(buf, b)
		if value == 0 {
			break
		}
	}
	return buf
}

func prependVarInt(buf []byte) []byte {
	length := len(buf)
	return append(appendVarInt(nil, length), buf...)
}

func readVarInt(r interface{ Read([]byte) (int, error) }) (int, error) {
	var result int
	var shift uint
	buf := make([]byte, 1)
	for {
		if _, err := r.Read(buf); err != nil {
			return 0, err
		}
		b := buf[0]
		result |= int(b&0x7F) << shift
		if b&0x80 == 0 {
			break
		}
		shift += 7
		if shift > 35 {
			return 0, fmt.Errorf("varint too big")
		}
	}
	return result, nil
}

func readFull(r interface{ Read([]byte) (int, error) }, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
