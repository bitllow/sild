import SwiftUI

/// The widget's Feather-style stroked glyphs, matched to SF Symbols at the same visual
/// weight (thin, round-capped) so the surfaces share one icon language.
enum SildIcon {
    case back, send, clip, arrow, chevron, close, speaker, speakerOff
}

struct SildIconView: View {
    let icon: SildIcon
    let size: CGFloat

    init(_ icon: SildIcon, size: CGFloat = 20) {
        self.icon = icon
        self.size = size
    }

    var body: some View {
        Image(systemName: symbol)
            .font(.system(size: size, weight: .regular))
            .imageScale(.medium)
            .frame(width: size, height: size)
            .accessibilityHidden(true)
    }

    private var symbol: String {
        switch icon {
        case .back: return "arrow.left"
        case .send: return "paperplane"
        case .clip: return "paperclip"
        case .arrow: return "arrow.right"
        case .chevron: return "chevron.right"
        case .close: return "xmark"
        case .speaker: return "speaker.wave.2"
        case .speakerOff: return "speaker.slash"
        }
    }
}
