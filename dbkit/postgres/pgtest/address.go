// SPDX-License-Identifier: Apache-2.0

package pgtest

// CheckAddress returns the error New refuses address with, or nil when New takes it.
func CheckAddress(address string) error {
	_, err := configOf(address)
	return err
}
