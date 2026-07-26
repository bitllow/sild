import SwiftUI
import SildCore

private let avatarPalette: [Color] = [
    0x3D63FF, 0xFF7A45, 0x18A957, 0x7C5CFF, 0x0EA5A5, 0xE0599B, 0xD9881A, 0x2440B8,
].map { hex in
    Color(
        .sRGB,
        red: Double((hex >> 16) & 0xFF) / 255,
        green: Double((hex >> 8) & 0xFF) / 255,
        blue: Double(hex & 0xFF) / 255,
        opacity: 1
    )
}

/// A single uppercase initial — the web widget's rowInitial/headInitial.
private func initial(_ name: String) -> String {
    guard let c = name.trimmingCharacters(in: .whitespaces).first else { return "?" }
    return String(c).uppercased()
}

private func colorFor(_ name: String) -> Color {
    var h = 0
    for ch in name.unicodeScalars { h = (h &* 31 &+ Int(ch.value)) & 0x7FFFFFFF }
    return avatarPalette[h % avatarPalette.count]
}

public struct SildAvatar: View {
    @Environment(\.sildStyle) private var style
    let name: String
    var size: CGFloat = 36
    var background: Color?

    public var body: some View {
        Circle()
            .fill(background ?? colorFor(name))
            .frame(width: size, height: size)
            .overlay(
                Text(initial(name))
                    .font(style.font(size * 0.38, .bold))
                    .foregroundStyle(.white)
            )
    }
}

/// The shared top-right cluster (sound + close), on every screen so the icons never
/// shift position.
public struct SildHeaderControls: View {
    @Environment(\.sildStyle) private var style
    let soundOn: Bool
    let onToggleSound: () -> Void
    let onClose: () -> Void

    public var body: some View {
        HStack(spacing: 2) {
            Button(action: onToggleSound) {
                SildIconView(soundOn ? .speaker : .speakerOff)
                    .foregroundStyle(style.colors.onBrand)
            }
            .accessibilityLabel(soundOn ? "Turn off reply notifications" : "Turn on reply notifications")
            Button(action: onClose) {
                SildIconView(.close).foregroundStyle(style.colors.onBrand)
            }
            .accessibilityLabel("Close")
        }
    }
}

/// The brand-colored thread top bar: back button, avatar, title + subtitle, and the
/// trailing control cluster.
public struct SildHeader<Action: View>: View {
    @Environment(\.sildStyle) private var style
    let title: String
    let subtitle: String?
    let onBack: (() -> Void)?
    var avatarName: String?
    @ViewBuilder let action: () -> Action

    public var body: some View {
        HStack(spacing: 10) {
            if let onBack {
                Button(action: onBack) {
                    SildIconView(.back).foregroundStyle(style.colors.onBrand)
                }
                .accessibilityLabel("Back")
            }
            if let avatarName {
                SildAvatar(name: avatarName, size: 36)
            }
            VStack(alignment: .leading, spacing: 1) {
                Text(title)
                    .font(style.font(16, .bold))
                    .foregroundStyle(style.colors.onBrand)
                    .lineLimit(1)
                if let subtitle, !subtitle.isEmpty {
                    Text(subtitle)
                        .font(style.font(12))
                        .foregroundStyle(style.colors.onBrand.opacity(0.8))
                        .lineLimit(1)
                }
            }
            Spacer(minLength: 0)
            action()
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 12)
        .background(style.colors.brand)
    }
}

/// One message: system lines centered; the visitor's own (OUT) messages right-aligned
/// in brand color; everyone else left-aligned with an author label. Inline images
/// render above the bubble, other files as tappable chips below it.
public struct SildMessageBubble: View {
    @Environment(\.sildStyle) private var style
    let message: Message
    let onOpenURL: (String) -> Void

    public var body: some View {
        if message.system {
            HStack {
                Spacer()
                Text(message.body)
                    .font(style.font(12))
                    .foregroundStyle(style.colors.tertiary)
                    .padding(.vertical, 4)
                Spacer()
            }
        } else {
            let out = message.direction == .out
            VStack(alignment: out ? .trailing : .leading, spacing: 0) {
                metaRow(out: out)
                ForEach(Array(inlineImages.enumerated()), id: \.offset) { _, att in
                    AsyncImage(url: URL(string: att.url ?? "")) { image in
                        image.resizable().scaledToFill()
                    } placeholder: {
                        style.colors.sunken
                    }
                    .frame(width: 220, height: 220)
                    .clipShape(RoundedRectangle(cornerRadius: style.radii.card))
                    .padding(.bottom, 4)
                }
                if !message.body.isEmpty {
                    Text(message.body)
                        .font(style.font(14))
                        .foregroundStyle(out ? style.colors.onBrand : style.colors.text)
                        .padding(.horizontal, 13)
                        .padding(.vertical, 9)
                        .background(out ? style.colors.brand : style.colors.sunken)
                        .clipShape(RoundedRectangle(cornerRadius: style.radii.bubble))
                }
                ForEach(Array(files.enumerated()), id: \.offset) { _, att in
                    fileChip(att)
                }
            }
            .frame(maxWidth: .infinity, alignment: out ? .trailing : .leading)
        }
    }

    // Author + time above the bubble (web parity); own messages are labelled "You".
    @ViewBuilder
    private func metaRow(out: Bool) -> some View {
        let label = out ? "You" : message.author
        if label != nil || !message.time.isEmpty {
            HStack(spacing: 7) {
                if let label {
                    Text(label)
                        .font(style.font(12, .semibold))
                        .foregroundStyle(style.colors.sub)
                }
                if !message.time.isEmpty {
                    Text(message.time)
                        .font(style.font(11))
                        .foregroundStyle(style.colors.tertiary)
                }
            }
            .padding(.horizontal, 4)
            .padding(.bottom, 4)
        }
    }

    private func fileChip(_ att: Attachment) -> some View {
        Button {
            if let url = att.url { onOpenURL(url) }
        } label: {
            HStack(spacing: 7) {
                SildIconView(.clip, size: 16).foregroundStyle(style.colors.tertiary)
                Text(att.filename.isEmpty ? "attachment" : att.filename)
                    .font(style.font(13))
                    .foregroundStyle(style.colors.text)
                    .lineLimit(1)
            }
            .padding(.horizontal, 11)
            .padding(.vertical, 8)
            .background(style.colors.card)
            .clipShape(RoundedRectangle(cornerRadius: style.radii.btn))
        }
        .buttonStyle(.plain)
        .disabled(att.url == nil)
        .padding(.top, 4)
    }

    private var inlineImages: [Attachment] { message.attachments.filter { $0.isInlineImage } }
    private var files: [Attachment] { message.attachments.filter { !$0.isInlineImage } }
}

/// Inline error line shown wherever the client surfaces a failure.
public struct SildErrorLine: View {
    @Environment(\.sildStyle) private var style
    let text: String

    public var body: some View {
        Text(text)
            .font(style.font(12))
            .foregroundStyle(style.colors.tertiary)
    }
}
