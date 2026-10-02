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

package msgconv

import (
	"context"
	"testing"

	"go.mau.fi/util/ptr"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-whatsapp/pkg/waid"
)

type uploadRecorder struct {
	bridgev2.MatrixAPI
	uploads map[string]string
}

func (u *uploadRecorder) UploadMedia(_ context.Context, _ id.RoomID, data []byte, fileName, _ string) (id.ContentURIString, *event.EncryptedFileInfo, error) {
	u.uploads[fileName] = string(data)
	return id.ContentURIString("mxc://example.com/" + fileName), nil, nil
}

func convertForTest(t *testing.T, msg *waE2E.Message) (*bridgev2.ConvertedMessage, map[string]string) {
	t.Helper()
	intent := &uploadRecorder{uploads: map[string]string{}}
	portal := &bridgev2.Portal{Portal: &database.Portal{MXID: "!room:example.com", Metadata: &waid.PortalMetadata{}}}
	info := &types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:   types.NewJID("5500000000001", types.DefaultUserServer),
			Sender: types.JID{User: "5500000000001", Server: types.DefaultUserServer, Device: 3},
		},
		ID: "3EB0CONTACTS",
	}
	cm := (&MessageConverter{}).ToMatrix(context.Background(), portal, nil, intent, msg, msg, info, false, false, nil)
	return cm, intent.uploads
}

func testContact(name string) *waE2E.ContactMessage {
	return &waE2E.ContactMessage{
		DisplayName: ptr.Ptr(name),
		Vcard:       ptr.Ptr("BEGIN:VCARD\nVERSION:3.0\nFN:" + name + "\nTEL;type=CELL:+55 21 90000-0001\nEND:VCARD"),
	}
}

func TestContactsArrayMessage(t *testing.T) {
	cm, uploads := convertForTest(t, &waE2E.Message{ContactsArrayMessage: &waE2E.ContactsArrayMessage{
		DisplayName: ptr.Ptr("2 contacts"),
		Contacts:    []*waE2E.ContactMessage{testContact("Ana Teste"), testContact("Bruno Teste")},
	}})
	if len(cm.Parts) != 2 {
		t.Fatalf("got %d parts, want one per contact", len(cm.Parts))
	}
	for i, want := range []struct {
		id   networkid.PartID
		name string
	}{{"", "Ana Teste.vcf"}, {"1", "Bruno Teste.vcf"}} {
		part := cm.Parts[i]
		if part.ID != want.id {
			t.Errorf("part %d: ID %q, want %q", i, part.ID, want.id)
		}
		if part.Content.MsgType != event.MsgFile || part.Content.FileName != want.name {
			t.Errorf("part %d: %s %q, want m.file %q", i, part.Content.MsgType, part.Content.FileName, want.name)
		}
		if uploads[want.name] == "" {
			t.Errorf("part %d: %s was not uploaded", i, want.name)
		}
		if part.Content.Mentions == nil {
			t.Errorf("part %d: mentions not set", i)
		}
		if meta, ok := part.DBMetadata.(*waid.MessageMetadata); !ok || meta.SenderDeviceID != 3 {
			t.Errorf("part %d: metadata %+v, want sender device 3", i, part.DBMetadata)
		}
	}
	if cm.Parts[0].DBMetadata == cm.Parts[1].DBMetadata {
		t.Error("parts share one metadata struct")
	}
}

func TestEmptyContactsArrayMessage(t *testing.T) {
	cm, _ := convertForTest(t, &waE2E.Message{ContactsArrayMessage: &waE2E.ContactsArrayMessage{}})
	if len(cm.Parts) != 1 || cm.Parts[0].Content.MsgType != event.MsgNotice {
		t.Fatalf("got %d parts, want one notice", len(cm.Parts))
	}
}

func TestSingleContactMessage(t *testing.T) {
	cm, uploads := convertForTest(t, &waE2E.Message{ContactMessage: testContact("Ana Teste")})
	if len(cm.Parts) != 1 || cm.Parts[0].ID != "" || cm.Parts[0].Content.FileName != "Ana Teste.vcf" || uploads["Ana Teste.vcf"] == "" {
		t.Fatalf("single contact: %d parts, first %+v", len(cm.Parts), cm.Parts[0].Content)
	}
}
