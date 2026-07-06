// webrtc_dep.go — isolated imports for webrtc.go so the latter
// stays focused on the state machine + handlers.
//
// Splitting the import here is purely a code-organisation
// choice — it lets a reader see "what packages does WebRTC code
// touch" at a glance without scrolling through dozens of
// unrelated imports.

package matching

import (
	cryptorand "crypto/rand"
)

// defaultRandRead is the production crypto/rand backend used
// by newSessionID. Wired through a var so a test could
// deterministically predict session ids if it needed to.
var defaultRandRead = cryptorand.Read
