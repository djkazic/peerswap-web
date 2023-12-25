// Code generated by go-swagger; DO NOT EDIT.

package models

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"
	"strconv"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
)

// PeerswapRequestSwapList peerswap request swap list
//
// swagger:model peerswapRequestSwapList
type PeerswapRequestSwapList struct {

	// requested swaps
	RequestedSwaps []*PeerswapRequestedSwap `json:"requestedSwaps"`
}

// Validate validates this peerswap request swap list
func (m *PeerswapRequestSwapList) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateRequestedSwaps(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *PeerswapRequestSwapList) validateRequestedSwaps(formats strfmt.Registry) error {
	if swag.IsZero(m.RequestedSwaps) { // not required
		return nil
	}

	for i := 0; i < len(m.RequestedSwaps); i++ {
		if swag.IsZero(m.RequestedSwaps[i]) { // not required
			continue
		}

		if m.RequestedSwaps[i] != nil {
			if err := m.RequestedSwaps[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("requestedSwaps" + "." + strconv.Itoa(i))
				} else if ce, ok := err.(*errors.CompositeError); ok {
					return ce.ValidateName("requestedSwaps" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// ContextValidate validate this peerswap request swap list based on the context it is used
func (m *PeerswapRequestSwapList) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateRequestedSwaps(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *PeerswapRequestSwapList) contextValidateRequestedSwaps(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(m.RequestedSwaps); i++ {

		if m.RequestedSwaps[i] != nil {

			if swag.IsZero(m.RequestedSwaps[i]) { // not required
				return nil
			}

			if err := m.RequestedSwaps[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("requestedSwaps" + "." + strconv.Itoa(i))
				} else if ce, ok := err.(*errors.CompositeError); ok {
					return ce.ValidateName("requestedSwaps" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

// MarshalBinary interface implementation
func (m *PeerswapRequestSwapList) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *PeerswapRequestSwapList) UnmarshalBinary(b []byte) error {
	var res PeerswapRequestSwapList
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}
