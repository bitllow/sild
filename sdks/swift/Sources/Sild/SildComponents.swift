import SwiftUI
import SildCore

struct SildAvatar: View {
    @Environment(\.sildStyle) private var style
    let name: String
    var size: CGFloat = 36
    var background: Color?

    var body: some View {
        Circle()
            .fill(background ?? SildColors.parse(BrandTheme.shared.avatarColor(name: name)))
            .frame(width: size, height: size)
            .overlay(
                Text(BrandTheme.shared.avatarInitial(name: name))
                    .font(style.font(size * 0.38, .bold))
                    .foregroundStyle(.white)
            )
    }
}

/// The shared top-right cluster (sound + close), on every screen so the icons never
/// shift position.
struct SildHeaderControls: View {
    @Environment(\.sildStyle) private var style
    @Environment(\.sildStrings) private var strings

    private func t(_ key: String, _ vars: [String: Any] = [:]) -> String { strings(key, vars) }
    let soundOn: Bool
    let onToggleSound: () -> Void
    let onClose: () -> Void

    var body: some View {
        HStack(spacing: 2) {
            Button(action: onToggleSound) {
                SildIconView(soundOn ? .speaker : .speakerOff)
                    .foregroundStyle(style.colors.onBrand)
            }
            .accessibilityLabel(t(soundOn ? "widget.notifications.disable" : "widget.notifications.enable"))
            Button(action: onClose) {
                SildIconView(.close).foregroundStyle(style.colors.onBrand)
            }
            .accessibilityLabel(t("widget.launcher.close"))
        }
    }
}

/// The brand-colored thread top bar: back button, avatar, title + subtitle, and the
/// trailing control cluster.
struct SildHeader: View {
    @Environment(\.sildStyle) private var style
    @Environment(\.sildStrings) private var strings

    private func t(_ key: String, _ vars: [String: Any] = [:]) -> String { strings(key, vars) }
    let title: String
    let subtitle: String
    let onBack: () -> Void
    let soundOn: Bool
    let onToggleSound: () -> Void
    let onClose: () -> Void

    var body: some View {
        HStack(spacing: 10) {
            Button(action: onBack) {
                SildIconView(.back).foregroundStyle(style.colors.onBrand)
            }
            .accessibilityLabel(t("widget.thread.back"))
            SildAvatar(name: title, size: 36)
            VStack(alignment: .leading, spacing: 1) {
                Text(title)
                    .font(style.font(16, .bold))
                    .foregroundStyle(style.colors.onBrand)
                    .lineLimit(1)
                if !subtitle.isEmpty {
                    Text(subtitle)
                        .font(style.font(12))
                        .foregroundStyle(style.colors.onBrand.opacity(0.8))
                        .lineLimit(1)
                }
            }
            Spacer(minLength: 0)
            SildHeaderControls(soundOn: soundOn, onToggleSound: onToggleSound, onClose: onClose)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 12)
        .background(style.colors.brand)
    }
}

/// One message: system lines centered; the visitor's own (OUT) messages right-aligned
/// in brand color; everyone else left-aligned with an author label. Inline images
/// render above the bubble, other files as tappable chips below it.
struct SildMessageBubble: View {
    @Environment(\.sildStyle) private var style
    @Environment(\.sildStrings) private var strings

    private func t(_ key: String, _ vars: [String: Any] = [:]) -> String { strings(key, vars) }
    let message: Message
    let onOpenURL: (String) -> Void

    var body: some View {
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
            // isInlineImage is a bridged getter, so split once rather than filter twice.
            let images = message.attachments.filter { $0.isInlineImage }
            let files = message.attachments.filter { !$0.isInlineImage }
            VStack(alignment: out ? .trailing : .leading, spacing: 0) {
                metaRow(out: out)
                ForEach(Array(images.enumerated()), id: \.offset) { _, att in
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
        let label = out ? t("widget.thread.you") : message.author
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
                Text(att.filename.isEmpty ? t("widget.composer.unnamedFile") : att.filename)
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

}

/// Inline error line shown wherever the client surfaces a failure.
struct SildErrorLine: View {
    @Environment(\.sildStyle) private var style
    let text: String

    var body: some View {
        Text(text)
            .font(style.font(12))
            .foregroundStyle(style.colors.tertiary)
    }
}
