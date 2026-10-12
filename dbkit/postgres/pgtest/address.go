// SPDX-License-Identifier: Apache-2.0

package pgtest

import (
	"fmt"
	"strings"

	"github.com/gopherium/framework/dbkit"
)

// poolPrefix starts every key pgxpool reads from an address.
const poolPrefix = "pool_"

// errPoolKey refuses an address that sets a pool_ key, which pgtestdb would send to the server as a setting.
var errPoolKey = fmt.Errorf("%w: the address sets a pool_ key, which pgtestdb would send to the server, "+
	"remove it and set pool keys on the address New returns", dbkit.ErrAddress)

// CheckAddress returns the error New refuses address with, or nil when New takes it.
func CheckAddress(address string) error {
	_, err := configOf(address)
	return err
}

// checkPoolKeys returns errPoolKey when params holds a key that starts with poolPrefix.
func checkPoolKeys(params map[string]string) error {
	for key := range params {
		if strings.HasPrefix(key, poolPrefix) {
			return errPoolKey
		}
	}
	return nil
}
