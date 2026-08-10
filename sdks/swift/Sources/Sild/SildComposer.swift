import SwiftUI
import SildCore

/// The message input row: a pending-attachment tray, an attach button, the text field
/// and a send button. Picked files show as removable chips (plus an "Uploading…" chip
/// while in flight) and are sent with the text; send is disabled until uploads settle.
///
/// The composer clears its input only once `onSend` reports success, so a failed send
/// keeps the draft — the same contract as the Android composer.
struct SildComposer: View {
    @Environment(\.sildStyle) private var style
    @Environment(\.sildStrings) private var strings

    private func t(_ key: String, _ vars: [String: Any] = [:]) -> String { strings(key, vars) }

    let pending: [PendingAttachment]
    let uploading: Int
    var enabled: Bool = true
    let onAttach: () -> Void
    let onRemove: (Int) -> Void
    let onSend: (String) async -> Bool

    @State private var text = ""
    @State private var sending = false

    private var canSend: Bool {
        enabled && !sending && uploading == 0 && (text.contains { !$0.isWhitespace } || !pending.isEmpty)
    }

    var body: some View {
        VStack(spacing: 0) {
            Rectangle().fill(style.colors.border).frame(height: 1)
            VStack(spacing: 0) {
                if !pending.isEmpty || uploading > 0 {
                    tray
                }
                inputRow
            }
            .padding(.horizontal, 12)
            .padding(.top, 10)
            .padding(.bottom, 12)
        }
        .background(style.colors.card)
    }

    private var tray: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 6) {
                ForEach(Array(pending.enumerated()), id: \.offset) { i, att in
                    HStack(spacing: 4) {
                        Text(att.filename.isEmpty ? t("widget.composer.unnamedFile") : att.filename)
                            .font(style.font(12))
                            .foregroundStyle(style.colors.sub)
                            .lineLimit(1)
                            .frame(maxWidth: 180)
                        Button { onRemove(i) } label: {
                            SildIconView(.close, size: 13).foregroundStyle(style.colors.tertiary)
                        }
                        .accessibilityLabel(t("widget.composer.remove"))
                    }
                    .padding(.leading, 10)
                    .padding(.trailing, 4)
                    .padding(.vertical, 5)
                    .background(style.colors.sunken)
                    .overlay(
                        RoundedRectangle(cornerRadius: style.radii.btn)
                            .stroke(style.colors.border, lineWidth: 1)
                    )
                    .clipShape(RoundedRectangle(cornerRadius: style.radii.btn))
                }
                if uploading > 0 {
                    Text(t("widget.composer.uploading"))
                        .font(style.font(12))
                        .foregroundStyle(style.colors.tertiary)
                }
            }
        }
        .padding(.bottom, 8)
    }

    private var inputRow: some View {
        HStack(alignment: .bottom, spacing: 8) {
            Button(action: onAttach) {
                SildIconView(.clip)
                    .foregroundStyle(style.colors.tertiary)
                    .frame(width: 34, height: 34)
            }
            .disabled(!enabled)
            .accessibilityLabel(t("widget.composer.attach"))

            TextField(t("widget.composer.placeholder"), text: $text, axis: .vertical)
                .font(style.font(14))
                .foregroundStyle(style.colors.text)
                .tint(style.colors.brand)
                .lineLimit(1...4)
                .disabled(!enabled)
                .padding(.vertical, 6)
                .accessibilityIdentifier("sild.composer.input")

            Button {
                let sent = text.trimmingCharacters(in: .whitespacesAndNewlines)
                sending = true
                Task {
                    let ok = await onSend(sent)
                    sending = false
                    // Clear only if the field still holds exactly what we sent — the input
                    // stays editable during the request, so anything typed meanwhile is the
                    // user's to keep (and a failure keeps the draft either way).
                    if ok, text.trimmingCharacters(in: .whitespacesAndNewlines) == sent { text = "" }
                }
            } label: {
                SildIconView(.send, size: 18)
                    .foregroundStyle(style.colors.onBrand)
                    .frame(width: 34, height: 34)
                    .opacity(canSend ? 1 : 0.4)
            }
            .buttonStyle(SildBrandButtonStyle(colors: style.colors, radius: style.radii.btn))
            .disabled(!canSend)
            .accessibilityIdentifier("sild.composer.send")
            .accessibilityLabel(t("widget.composer.send"))
        }
        .padding(.leading, 8)
        .padding(.trailing, 6)
        .padding(.vertical, 6)
        .overlay(
            RoundedRectangle(cornerRadius: style.radii.card)
                .stroke(style.colors.border, lineWidth: 1)
        )
    }
}
