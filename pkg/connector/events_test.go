// mautrix-whatsapp - A Matrix-WhatsApp puppeting bridge.
// Copyright (C) 2026 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package connector

import (
	"context"
	"slices"
	"testing"
	"time"

	"go.mau.fi/util/ptr"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-whatsapp/pkg/msgconv"
	"go.mau.fi/mautrix-whatsapp/pkg/waid"
)

type uploadStub struct {
	bridgev2.MatrixAPI
}

func (uploadStub) UploadMedia(_ context.Context, _ id.RoomID, _ []byte, fileName, _ string) (id.ContentURIString, *event.EncryptedFileInfo, error) {
	return id.ContentURIString("mxc://example.com/" + fileName), nil, nil
}

func testContact(name string) *waE2E.ContactMessage {
	return &waE2E.ContactMessage{
		DisplayName: ptr.Ptr(name),
		Vcard:       ptr.Ptr("BEGIN:VCARD\nVERSION:3.0\nFN:" + name + "\nTEL;type=CELL:+55 21 90000-0001\nEND:VCARD"),
	}
}

func TestConvertEditDecryptedMultiPart(t *testing.T) {
	contactsArray := &waE2E.Message{ContactsArrayMessage: &waE2E.ContactsArrayMessage{
		Contacts: []*waE2E.ContactMessage{testContact("Ana Teste"), testContact("Bruno Teste"), testContact("Carla Teste")},
		ContextInfo: &waE2E.ContextInfo{
			StanzaID:   ptr.Ptr("3EB0QUOTED"),
			Expiration: ptr.Ptr(uint32(86400)),
		},
	}}
	tests := []struct {
		name          string
		msg           *waE2E.Message
		existingParts int
		wantAdded     []networkid.PartID
	}{
		{"contact array replacing error notice", contactsArray, 1, []networkid.PartID{"1", "2"}},
		{"contact array already fully bridged", contactsArray, 3, nil},
		{"single contact", &waE2E.Message{ContactMessage: testContact("Ana Teste")}, 1, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			portal := &bridgev2.Portal{Portal: &database.Portal{MXID: "!room:example.com", Metadata: &waid.PortalMetadata{}}}
			info := types.MessageInfo{
				MessageSource: types.MessageSource{
					Chat:   types.NewJID("5500000000001", types.DefaultUserServer),
					Sender: types.JID{User: "5500000000001", Server: types.DefaultUserServer, Device: 3},
				},
				ID: "3EB0CONTACTS",
			}
			wa := &WhatsAppClient{Main: &WhatsAppConnector{MsgConv: &msgconv.MessageConverter{}, mediaEditCache: make(MediaEditCache)}}
			evt := &WAMessageEvent{
				MessageInfoWrapper:            &MessageInfoWrapper{Info: info, wa: wa},
				Message:                       tt.msg,
				MsgEvent:                      &events.Message{Info: info, Message: tt.msg, RawMessage: tt.msg},
				isUndecryptableUpsertSubEvent: true,
			}
			var existing []*database.Message
			for i := range tt.existingParts {
				existing = append(existing, &database.Message{
					ID:       evt.GetID(),
					PartID:   waid.MakeMessagePartID(i),
					MXID:     id.EventID("$part" + string(waid.MakeMessagePartID(i))),
					Metadata: &waid.MessageMetadata{Error: waid.MsgErrDecryptionFailed},
				})
			}
			decrypted := &WANowDecryptableMessage{WAMessageEvent: evt, editParts: existing}

			converted, err := decrypted.ConvertEdit(context.Background(), portal, uploadStub{}, decrypted.GetTargetDBMessage())
			if err != nil {
				t.Fatal(err)
			}
			if len(converted.ModifiedParts) != 1 || converted.ModifiedParts[0].Part != existing[0] {
				t.Fatalf("modified parts: %+v, want first existing part", converted.ModifiedParts)
			}
			if converted.ModifiedParts[0].Part.Metadata.(*waid.MessageMetadata).Error != "" {
				t.Error("decryption error was not cleared from the edited part")
			}
			if tt.wantAdded == nil {
				if converted.AddedParts != nil {
					t.Fatalf("added %d parts, want none", len(converted.AddedParts.Parts))
				}
				return
			}
			if converted.AddedParts == nil {
				t.Fatalf("no added parts, want %v", tt.wantAdded)
			}
			var gotAdded []networkid.PartID
			for _, part := range converted.AddedParts.Parts {
				gotAdded = append(gotAdded, part.ID)
			}
			if !slices.Equal(gotAdded, tt.wantAdded) {
				t.Fatalf("added parts %v, want %v", gotAdded, tt.wantAdded)
			}
			if name := converted.AddedParts.Parts[1].Content.FileName; name != "Carla Teste.vcf" {
				t.Errorf("last added part is %q, want Carla Teste.vcf", name)
			}
			if converted.AddedParts.ReplyTo == nil || converted.AddedParts.Disappear.Timer != 24*time.Hour {
				t.Errorf("added parts lost reply %v or disappearing timer %v", converted.AddedParts.ReplyTo, converted.AddedParts.Disappear.Timer)
			}
		})
	}
}
