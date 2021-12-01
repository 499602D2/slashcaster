package api

// Internal typedef
type Slashing struct {
	AttestationViolation bool
	ProposerViolation    bool
	ValidatorIndex       string
	Slot                 string
}

// Internal typedef
type SlashingEvent struct {
	Slashings     []Slashing
	AttSlashings  int
	PropSlashings int
	Slot          string
}

// Infura's data is unpacked here
type BlockData struct {
	Block Block `json:"data"`
}

// Represents a block, with a message and a signature
type Block struct {
	Message   BlockMessage `json:"message"`
	Signature string       `json:"signature"`
}

// Message within block
type BlockMessage struct {
	Slot          string    `json:"slot"`
	ProposerIndex string    `json:"proposer_index"`
	Body          BlockBody `json:"body"`
}

// Block body within the message
type BlockBody struct {
	ProposerSlashings []ProposerViolation    `json:"proposer_slashings"`
	AttesterSlashings []AttestationViolation `json:"attester_slashings"`
}

// A slashing caused by an invalid proposal
type ProposerViolation struct {
	SignedHeader1 SignedBlockHeader `json:"signed_header_1"`
	SignedHeader2 SignedBlockHeader `json:"signed_header_2"`
}

// A slashing caused by an invalid attestations
type AttestationViolation struct {
	Attestation1 Attestation `json:"attestation_1"`
	Attestation2 Attestation `json:"attestation_2"`
}

// Block header with the message
type SignedBlockHeader struct {
	Message Message `json:"message"`
}

// A single attestation
type Attestation struct {
	AttestingIndices []string `json:"attesting_indices"`
}

// Message within a block header
type Message struct {
	Slot          string `json:"slot"`
	ProposerIndex string `json:"proposer_index"`
}

// For getting chain head
type HeadData struct {
	HeadData ChainHead `json:"data"`
}
type ChainHead struct {
	// https://infura.io/docs/eth2#operation/getEthV1NodeSyncing
	HeadSlot  string `json:"head_slot"`
	SyncDist  string `json:"sync_distance"`
	IsSyncing bool   `json:"is_syncing"`
}
