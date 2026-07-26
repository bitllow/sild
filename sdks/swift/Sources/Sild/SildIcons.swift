import SwiftUI

/// The widget's Feather-style stroked glyphs. SF Symbols cover most of them at the
/// same visual weight (thin, round-capped); the two that have no close match — the
/// send paper-plane outline and the muted speaker — are drawn from the same SVG path
/// data the web drop-in uses, so all three surfaces share one icon language.
public enum SildIcon {
    case back, send, clip, arrow, chevron, close, speaker, speakerOff
}

public struct SildIconView: View {
    let icon: SildIcon
    var size: CGFloat = 20

    public init(_ icon: SildIcon, size: CGFloat = 20) {
        self.icon = icon
        self.size = size
    }

    public var body: some View {
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
