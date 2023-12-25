// Code generated by go-swagger; DO NOT EDIT.

package peer_swap

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"
	"io"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"

	"peerswap-web/models"
)

// PeerSwapListRequestedSwapsReader is a Reader for the PeerSwapListRequestedSwaps structure.
type PeerSwapListRequestedSwapsReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *PeerSwapListRequestedSwapsReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {
	case 200:
		result := NewPeerSwapListRequestedSwapsOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil
	default:
		result := NewPeerSwapListRequestedSwapsDefault(response.Code())
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		if response.Code()/100 == 2 {
			return result, nil
		}
		return nil, result
	}
}

// NewPeerSwapListRequestedSwapsOK creates a PeerSwapListRequestedSwapsOK with default headers values
func NewPeerSwapListRequestedSwapsOK() *PeerSwapListRequestedSwapsOK {
	return &PeerSwapListRequestedSwapsOK{}
}

/*
PeerSwapListRequestedSwapsOK describes a response with status code 200, with default header values.

A successful response.
*/
type PeerSwapListRequestedSwapsOK struct {
	Payload *models.PeerswapListRequestedSwapsResponse
}

// IsSuccess returns true when this peer swap list requested swaps o k response has a 2xx status code
func (o *PeerSwapListRequestedSwapsOK) IsSuccess() bool {
	return true
}

// IsRedirect returns true when this peer swap list requested swaps o k response has a 3xx status code
func (o *PeerSwapListRequestedSwapsOK) IsRedirect() bool {
	return false
}

// IsClientError returns true when this peer swap list requested swaps o k response has a 4xx status code
func (o *PeerSwapListRequestedSwapsOK) IsClientError() bool {
	return false
}

// IsServerError returns true when this peer swap list requested swaps o k response has a 5xx status code
func (o *PeerSwapListRequestedSwapsOK) IsServerError() bool {
	return false
}

// IsCode returns true when this peer swap list requested swaps o k response a status code equal to that given
func (o *PeerSwapListRequestedSwapsOK) IsCode(code int) bool {
	return code == 200
}

// Code gets the status code for the peer swap list requested swaps o k response
func (o *PeerSwapListRequestedSwapsOK) Code() int {
	return 200
}

func (o *PeerSwapListRequestedSwapsOK) Error() string {
	return fmt.Sprintf("[GET /v1/swaps/requests][%d] peerSwapListRequestedSwapsOK  %+v", 200, o.Payload)
}

func (o *PeerSwapListRequestedSwapsOK) String() string {
	return fmt.Sprintf("[GET /v1/swaps/requests][%d] peerSwapListRequestedSwapsOK  %+v", 200, o.Payload)
}

func (o *PeerSwapListRequestedSwapsOK) GetPayload() *models.PeerswapListRequestedSwapsResponse {
	return o.Payload
}

func (o *PeerSwapListRequestedSwapsOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.PeerswapListRequestedSwapsResponse)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}

// NewPeerSwapListRequestedSwapsDefault creates a PeerSwapListRequestedSwapsDefault with default headers values
func NewPeerSwapListRequestedSwapsDefault(code int) *PeerSwapListRequestedSwapsDefault {
	return &PeerSwapListRequestedSwapsDefault{
		_statusCode: code,
	}
}

/*
PeerSwapListRequestedSwapsDefault describes a response with status code -1, with default header values.

An unexpected error response.
*/
type PeerSwapListRequestedSwapsDefault struct {
	_statusCode int

	Payload *models.RPCStatus
}

// IsSuccess returns true when this peer swap list requested swaps default response has a 2xx status code
func (o *PeerSwapListRequestedSwapsDefault) IsSuccess() bool {
	return o._statusCode/100 == 2
}

// IsRedirect returns true when this peer swap list requested swaps default response has a 3xx status code
func (o *PeerSwapListRequestedSwapsDefault) IsRedirect() bool {
	return o._statusCode/100 == 3
}

// IsClientError returns true when this peer swap list requested swaps default response has a 4xx status code
func (o *PeerSwapListRequestedSwapsDefault) IsClientError() bool {
	return o._statusCode/100 == 4
}

// IsServerError returns true when this peer swap list requested swaps default response has a 5xx status code
func (o *PeerSwapListRequestedSwapsDefault) IsServerError() bool {
	return o._statusCode/100 == 5
}

// IsCode returns true when this peer swap list requested swaps default response a status code equal to that given
func (o *PeerSwapListRequestedSwapsDefault) IsCode(code int) bool {
	return o._statusCode == code
}

// Code gets the status code for the peer swap list requested swaps default response
func (o *PeerSwapListRequestedSwapsDefault) Code() int {
	return o._statusCode
}

func (o *PeerSwapListRequestedSwapsDefault) Error() string {
	return fmt.Sprintf("[GET /v1/swaps/requests][%d] PeerSwap_ListRequestedSwaps default  %+v", o._statusCode, o.Payload)
}

func (o *PeerSwapListRequestedSwapsDefault) String() string {
	return fmt.Sprintf("[GET /v1/swaps/requests][%d] PeerSwap_ListRequestedSwaps default  %+v", o._statusCode, o.Payload)
}

func (o *PeerSwapListRequestedSwapsDefault) GetPayload() *models.RPCStatus {
	return o.Payload
}

func (o *PeerSwapListRequestedSwapsDefault) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	o.Payload = new(models.RPCStatus)

	// response payload
	if err := consumer.Consume(response.Body(), o.Payload); err != nil && err != io.EOF {
		return err
	}

	return nil
}
