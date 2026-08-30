package sessiondistill

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
)

func sealBundle(result Distillation, jsonOutput, markdownOutput []byte) Bundle {
	bundle := Bundle{
		result:   cloneDistillation(result),
		json:     append([]byte(nil), jsonOutput...),
		markdown: append([]byte(nil), markdownOutput...),
	}
	bundle.proof = bundleProof(bundle.json, bundle.markdown)
	return bundle
}

func validBundle(bundle Bundle) bool {
	if bundle.proof != bundleProof(bundle.json, bundle.markdown) {
		return false
	}
	if !validModelAssistance(bundle.result) {
		return false
	}
	jsonOutput, err := renderJSON(bundle.result)
	if err != nil || !bytes.Equal(jsonOutput, bundle.json) {
		return false
	}
	markdownOutput, err := renderMarkdown(bundle.result)
	return err == nil && bytes.Equal(markdownOutput, bundle.markdown)
}

// Result returns a deep copy of the verified structured distillation. Mutating
// the returned value cannot change the opaque bundle accepted by PublishBundle.
func (bundle Bundle) Result() Distillation {
	return cloneDistillation(bundle.result)
}

// JSON returns a copy of the verified JSON report.
func (bundle Bundle) JSON() []byte {
	return append([]byte(nil), bundle.json...)
}

// Markdown returns a copy of the verified Markdown report.
func (bundle Bundle) Markdown() []byte {
	return append([]byte(nil), bundle.markdown...)
}

func cloneDistillation(input Distillation) Distillation {
	output := input
	output.UserItems = cloneItems(input.UserItems)
	output.AssistantContext = cloneItems(input.AssistantContext)
	if input.Relations != nil {
		output.Relations = make([]Relation, len(input.Relations))
		copy(output.Relations, input.Relations)
	}
	if input.ModelAssistance != nil {
		assistance := *input.ModelAssistance
		if input.ModelAssistance.RankedItemIDs != nil {
			assistance.RankedItemIDs = append([]string{}, input.ModelAssistance.RankedItemIDs...)
		}
		output.ModelAssistance = &assistance
	}
	return output
}

func cloneItems(input []Item) []Item {
	if input == nil {
		return nil
	}
	output := make([]Item, len(input))
	for index := range input {
		output[index] = input[index]
		if input[index].Sources != nil {
			output[index].Sources = make([]Source, len(input[index].Sources))
			copy(output[index].Sources, input[index].Sources)
		}
	}
	return output
}

func bundleProof(jsonOutput, markdownOutput []byte) [32]byte {
	hash := sha256.New()
	hash.Write([]byte("mindweaver/sessiondistill/bundle/v1"))
	hash.Write([]byte{0})
	var length [8]byte
	for _, value := range [][]byte{jsonOutput, markdownOutput} {
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		hash.Write(length[:])
		hash.Write(value)
	}
	var result [32]byte
	copy(result[:], hash.Sum(nil))
	return result
}
