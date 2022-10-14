package transport

import (
	"errors"
	"github.com/oringik/crypto-chateau/aes-256"
	"net"
	"time"
)

type Conn struct {
	tcpConn      net.Conn
	reservedData []byte
	cfg          connCfg
	encryption   encryption
}

type connCfg struct {
	writeDeadline time.Duration
	readDeadline  time.Duration
}

type encryption struct {
	enabled   bool
	sharedKey []byte
}

func newConn(tcpConn net.Conn, cfg connCfg) *Conn {
	return &Conn{
		tcpConn: tcpConn,
		cfg:     cfg,
	}
}

func (cn *Conn) enableEncryption(sharedKey [32]byte) error {
	if cn.encryption.enabled {
		return errors.New("encryption already enabled")
	}

	sharedKeyHash, err := getSha256FromBytes(sharedKey)
	if err != nil {
		return err
	}

	cn.encryption.enabled = true
	cn.encryption.sharedKey = sharedKeyHash

	return nil
}

func (cn *Conn) Write(p []byte) (int, error) {
	if cn.cfg.writeDeadline > 0 {
		err := cn.SetWriteDeadline(time.Now().Add(cn.cfg.writeDeadline))
		if err != nil {
			return 0, err
		}
	}

	data := make([]byte, 0, len(p))

	if cn.encryption.enabled {
		encryptedData, err := aes_256.Encrypt(p, cn.encryption.sharedKey)
		if err != nil {
			return 0, err
		}

		data = encryptedData
	} else {
		data = p
	}

	dataWithLength := make([]byte, 0, len(data)+2)
	convertedLength := uint16(len(data))
	dataWithLength = append(dataWithLength, byte(convertedLength), byte(convertedLength>>8))
	dataWithLength = append(dataWithLength, data...)
	n, err := cn.tcpConn.Write(dataWithLength)
	return n, err
}

func (cn *Conn) Read(p []byte) (int, error) {
	if cn.cfg.readDeadline > 0 {
		err := cn.SetReadDeadline(time.Now().Add(cn.cfg.readDeadline))
		if err != nil {
			return 0, err
		}
	}

	buf := make([]byte, 2+len(p))

	if len(cn.reservedData) > 0 {
		if len(cn.reservedData) < 2 {
			return 0, errors.New("not enough length of data for getting packet length")
		}

		packetLength := uint16(cn.reservedData[0]) | uint16(cn.reservedData[1])<<8

		if len(cn.reservedData) < int(2+packetLength) {
			return 0, errors.New("incorrect packet length")
		}
		buf = cn.reservedData[:2+packetLength]

		if int(2+packetLength) < len(cn.reservedData) {
			cn.reservedData = cn.reservedData[2+packetLength:]
		}

		if int(2+packetLength) < len(buf) {
			reserved := buf[2+packetLength:]
			cn.reservedData = append(cn.reservedData, reserved...)
		}

		var data []byte

		if cn.encryption.enabled {
			decryptedData, err := aes_256.Decrypt(buf, cn.encryption.sharedKey)
			if err != nil {
				return 0, err
			}

			data = decryptedData
		} else {
			data = buf
		}

		copy(p, data)

		return len(data), nil
	}

	n, err := cn.tcpConn.Read(buf)
	if err != nil {
		return 0, err
	}

	if len(buf) <= 2 {
		return 0, errors.New("not enough length of data for getting packet length")
	}

	packetLength := uint16(buf[0]) | uint16(buf[1])<<8

	var packet []byte
	var toReserve []byte

	if int(packetLength) > n {
		packet = buf[2:n]
	} else {
		packet = buf[2 : 2+packetLength]
		if len(buf) > 1+int(packetLength) {
			toReserve = buf[2+packetLength:]
		}
	}

	cn.reservedData = append(cn.reservedData, toReserve...)

	var data []byte

	if cn.encryption.enabled {
		decryptedData, err := aes_256.Decrypt(packet, cn.encryption.sharedKey)
		if err != nil {
			return 0, err
		}

		data = decryptedData
	} else {
		data = packet
	}

	copy(p, data)

	return len(data), nil

}

func (cn *Conn) Close() error {
	return cn.tcpConn.Close()
}

func (cn *Conn) LocalAddr() net.Addr {
	return cn.tcpConn.LocalAddr()
}

func (cn *Conn) RemoteAddr() net.Addr {
	return cn.tcpConn.RemoteAddr()
}

func (cn *Conn) SetDeadline(t time.Time) error {
	return cn.tcpConn.SetDeadline(t)
}

func (cn *Conn) SetReadDeadline(t time.Time) error {
	return cn.tcpConn.SetReadDeadline(t)
}

func (cn *Conn) SetWriteDeadline(t time.Time) error {
	return cn.tcpConn.SetWriteDeadline(t)
}
