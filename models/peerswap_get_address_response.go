// Code generated by go-swagger; DO NOT EDIT.

package models

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"

	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
)

// PeerswapGetAddressResponse peerswap get address response
//
// swagger:model peerswapGetAddressResponse
type PeerswapGetAddressResponse struct {

	// address
	Address string `json:"address,omitempty"`
}

// Validate validates this peerswap get address response
func (m *PeerswapGetAddressResponse) Validate(formats strfmt.Registry) error {
	return nil
}

// ContextValidate validates this peerswap get address response based on context it is used
func (m *PeerswapGetAddressResponse) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	return nil
}

// MarshalBinary interface implementation
func (m *PeerswapGetAddressResponse) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *PeerswapGetAddressResponse) UnmarshalBinary(b []byte) error {
	var res PeerswapGetAddressResponse
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}
