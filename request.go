package ipp

import (
	"bytes"
	"encoding/binary"
	"io"
)

// Request defines a ipp request
type Request struct {
	ProtocolVersionMajor int8
	ProtocolVersionMinor int8

	Operation int16
	RequestId int32

	OperationAttributes map[string]interface{}
	JobAttributes       map[string]interface{}
	PrinterAttributes   map[string]interface{}

	File     io.Reader
	FileSize int
}

// NewRequest creates a new ipp request
func NewRequest(op int16, reqID int32) *Request {
	return &Request{
		ProtocolVersionMajor: ProtocolVersionMajor,
		ProtocolVersionMinor: ProtocolVersionMinor,
		Operation:            op,
		RequestId:            reqID,
		OperationAttributes:  make(map[string]interface{}),
		JobAttributes:        make(map[string]interface{}),
		PrinterAttributes:    make(map[string]interface{}),
		File:                 nil,
		FileSize:             -1,
	}
}

// Encode encodes the request to a byte slice
func (r *Request) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	enc := NewAttributeEncoder(buf)

	if err := binary.Write(buf, binary.BigEndian, r.ProtocolVersionMajor); err != nil {
		return nil, err
	}

	if err := binary.Write(buf, binary.BigEndian, r.ProtocolVersionMinor); err != nil {
		return nil, err
	}

	if err := binary.Write(buf, binary.BigEndian, r.Operation); err != nil {
		return nil, err
	}

	if err := binary.Write(buf, binary.BigEndian, r.RequestId); err != nil {
		return nil, err
	}

	if err := binary.Write(buf, binary.BigEndian, int8(TagOperation)); err != nil {
		return nil, err
	}

	if err := enc.Encode(AttributeCharset, Charset); err != nil {
		return nil, err
	}

	if err := enc.Encode(AttributeNaturalLanguage, CharsetLanguage); err != nil {
		return nil, err
	}

	if len(r.OperationAttributes) > 0 {
		if err := r.encodeOperationAttributes(enc); err != nil {
			return nil, err
		}
	}

	if len(r.JobAttributes) > 0 {
		if err := binary.Write(buf, binary.BigEndian, int8(TagJob)); err != nil {
			return nil, err
		}
		for attr, value := range r.JobAttributes {
			if err := enc.Encode(attr, value); err != nil {
				return nil, err
			}
		}
	}

	if len(r.PrinterAttributes) > 0 {
		if err := binary.Write(buf, binary.BigEndian, int8(TagPrinter)); err != nil {
			return nil, err
		}
		for attr, value := range r.PrinterAttributes {
			if err := enc.Encode(attr, value); err != nil {
				return nil, err
			}
		}
	}

	if err := binary.Write(buf, binary.BigEndian, int8(TagEnd)); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (r *Request) encodeOperationAttributes(enc *AttributeEncoder) error {
	ordered := []string{
		AttributeCharset,
		AttributeNaturalLanguage,
		AttributePrinterURI,
		AttributeJobID,
	}

	for _, attr := range ordered {
		if value, ok := r.OperationAttributes[attr]; ok {
			delete(r.OperationAttributes, attr)
			if err := enc.Encode(attr, value); err != nil {
				return err
			}
		}
	}

	for attr, value := range r.OperationAttributes {
		if err := enc.Encode(attr, value); err != nil {
			return err
		}
	}
	return nil
}

// RequestDecoder reads and decodes a request from a stream
type RequestDecoder struct {
	reader io.Reader
}

// NewRequestDecoder returns a new decoder that reads from r
func NewRequestDecoder(r io.Reader) *RequestDecoder {
	return &RequestDecoder{
		reader: r,
	}
}

// Decode decodes a ipp request into a request  struct. additional data will be written to an io.Writer if data is not nil
func (d *RequestDecoder) Decode(data io.Writer) (*Request, error) {
	req := new(Request)

	if err := binary.Read(d.reader, binary.BigEndian, &req.ProtocolVersionMajor); err != nil {
		return nil, err
	}

	if err := binary.Read(d.reader, binary.BigEndian, &req.ProtocolVersionMinor); err != nil {
		return nil, err
	}

	if err := binary.Read(d.reader, binary.BigEndian, &req.Operation); err != nil {
		return nil, err
	}

	if err := binary.Read(d.reader, binary.BigEndian, &req.RequestId); err != nil {
		return nil, err
	}

	startByteSlice := make([]byte, 1)

	tag := TagCupsInvalid
	previousAttributeName := ""
	tagSet := false

	attribDecoder := NewAttributeDecoder(d.reader)

	// decode attribute buffer
	for {
		if _, err := d.reader.Read(startByteSlice); err != nil {
			// when we read from a stream, we may get an EOF if we want to read the end tag
			// all data should be read and we can ignore the error
			if err == io.EOF {
				break
			}
			return nil, err
		}

		startByte := int8(startByteSlice[0])

		// check if attributes are completed
		if startByte == TagEnd {
			break
		}

		if startByte == TagOperation {
			if req.OperationAttributes == nil {
				req.OperationAttributes = make(map[string]interface{})
			}

			tag = TagOperation
			tagSet = true

		}

		if startByte == TagJob {
			if req.JobAttributes == nil {
				req.JobAttributes = make(map[string]interface{})
			}
			tag = TagJob
			tagSet = true
		}

		if startByte == TagPrinter {
			if req.PrinterAttributes == nil {
				req.PrinterAttributes = make(map[string]interface{})
			}
			tag = TagPrinter
			tagSet = true
		}

		if tagSet {
			if _, err := d.reader.Read(startByteSlice); err != nil {
				return nil, err
			}
			startByte = int8(startByteSlice[0])
		}

		attrib, err := attribDecoder.Decode(startByte)
		if err != nil {
			return nil, err
		}

		if attrib.Name != "" {
			appendAttributeToRequest(req, tag, attrib.Name, attrib.Value)
			previousAttributeName = attrib.Name
		} else {
			appendAttributeToRequest(req, tag, previousAttributeName, attrib.Value)
		}

		tagSet = false
	}

	if data != nil {
		if _, err := io.Copy(data, d.reader); err != nil {
			return nil, err
		}
	}

	return req, nil
}

func appendAttributeToRequest(req *Request, tag int8, name string, value interface{}) {
	switch tag {
	case TagOperation:
		req.OperationAttributes[name] = value
	case TagPrinter:
		req.PrinterAttributes[name] = value
	case TagJob:
		req.JobAttributes[name] = value
	}
}
