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

// WhatsApp's vcard strings end at END:VCARD with no trailing newline.
func testVcard(name, tel string) string {
	return "BEGIN:VCARD\nVERSION:3.0\nN:;" + name + ";;;\nFN:" + name + "\nitem1.TEL;waid=" + tel + ":+" + tel + "\nitem1.X-ABLabel:Mobile\nEND:VCARD"
}

func TestContactsArrayMessage(t *testing.T) {
	ana, bruno := testVcard("Ana Teste", "5521900000001"), testVcard("Bruno Teste", "5521900000002")
	for _, tc := range []struct{ name, displayName, wantFile string }{
		{"display name", "Ana Teste and 1 other contact", "Ana Teste and 1 other contact.vcf"},
		{"no display name", "", "2 contacts.vcf"},
	} {
		cm, uploads := convertForTest(t, &waE2E.Message{ContactsArrayMessage: &waE2E.ContactsArrayMessage{
			DisplayName: ptr.Ptr(tc.displayName),
			Contacts: []*waE2E.ContactMessage{
				{DisplayName: ptr.Ptr("Ana Teste"), Vcard: ptr.Ptr(ana)},
				{DisplayName: ptr.Ptr("Bruno Teste"), Vcard: ptr.Ptr(bruno)},
			},
		}})
		if len(cm.Parts) != 1 {
			t.Fatalf("%s: got %d parts, want 1", tc.name, len(cm.Parts))
		}
		content := cm.Parts[0].Content
		if content.MsgType != event.MsgFile || content.FileName != tc.wantFile {
			t.Fatalf("%s: got %s %q, want m.file %q", tc.name, content.MsgType, content.FileName, tc.wantFile)
		}
		want := ana + "\n" + bruno
		if got := uploads[tc.wantFile]; got != want {
			t.Fatalf("%s: uploaded %q, want %q", tc.name, got, want)
		}
		if content.Info.Size != len(want) {
			t.Fatalf("%s: size %d, want %d", tc.name, content.Info.Size, len(want))
		}
	}
}

func TestSingleContactMessage(t *testing.T) {
	ana := testVcard("Ana Teste", "5521900000001")
	cm, uploads := convertForTest(t, &waE2E.Message{ContactMessage: &waE2E.ContactMessage{DisplayName: ptr.Ptr("Ana Teste"), Vcard: ptr.Ptr(ana)}})
	if len(cm.Parts) != 1 || cm.Parts[0].Content.FileName != "Ana Teste.vcf" || uploads["Ana Teste.vcf"] != ana || cm.Parts[0].Content.Info.Size != len(ana) {
		t.Fatalf("single contact changed: %d parts, first %+v", len(cm.Parts), cm.Parts[0].Content)
	}
}
