import SwiftUI
import PhotosUI
import UniformTypeIdentifiers
import SildCore

/// The support landing — the counterpart of the web widget's Home: a brand-colored
/// welcome header (logo/name, "agents online", heading, sub), a "Send us a message"
/// card, quick-start topics, and a Recent list. A support request is created lazily on
/// the first message, so opening support without a conversation lands here.
struct HomeScreen: View {
    @Environment(\.sildStyle) private var style
    @Environment(\.sildStrings) private var strings
    @Environment(\.sildPlurals) private var plurals

    private func t(_ key: String, _ vars: [String: Any] = [:]) -> String { strings(key, vars) }

    // The count matches the avatars beside it.
    private let teamOnline: Int32 = 2

    let state: SildState
    let onNew: () -> Void
    let onOpen: (String) -> Void
    let onToggleSound: () -> Void
    let onClose: () -> Void

    var body: some View {
        // topicList re-splits the config string and bridges a new array on each read.
        let topics = state.brand.topicList
        VStack(spacing: 0) {
            header
            ScrollView {
                VStack(alignment: .leading, spacing: 12) {
                    newConversationCard
                    ForEach(Array(topics.enumerated()), id: \.offset) { _, topic in
                        topicRow(topic)
                    }
                    if !state.conversations.isEmpty {
                        recentList
                    }
                    if let error = state.error {
                        SildErrorLine(text: error)
                    }
                    if state.brand.poweredBy {
                        Text("Powered by Sild")
                            .font(style.font(11))
                            .foregroundStyle(style.colors.tertiary)
                            .frame(maxWidth: .infinity)
                            .padding(.top, 8)
                    }
                }
                .padding(16)
            }
        }
        .background(style.colors.page)
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack {
                if let logo = state.brand.logoSrc, let url = URL(string: logo) {
                    AsyncImage(url: url) { $0.resizable().scaledToFit() } placeholder: { Color.clear }
                        .frame(height: 28)
                } else if !state.brandName.isEmpty {
                    Text(state.brandName)
                        .font(style.font(18, .heavy))
                        .foregroundStyle(style.colors.onBrand)
                }
                Spacer()
                SildHeaderControls(soundOn: state.soundOn, onToggleSound: onToggleSound, onClose: onClose)
            }
            if state.brand.showTeam {
                teamRow.padding(.top, 14)
            }
            Text(state.brand.heading)
                .font(style.font(26, .heavy))
                .foregroundStyle(style.colors.onBrand)
                .padding(.top, 14)
            if !state.brand.sub.isEmpty {
                Text(state.brand.sub)
                    .font(style.font(14))
                    .foregroundStyle(style.colors.onBrand.opacity(0.9))
                    .padding(.top, 6)
            }
        }
        .padding(20)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(style.colors.brand)
    }

    // Mirrors the web widget's "agents online" flourish (shown when showTeam).
    private var teamRow: some View {
        HStack(spacing: 0) {
            ZStack(alignment: .leading) {
                teamAvatar("E", Color(.sRGB, red: 0.486, green: 0.612, blue: 0.961, opacity: 1))
                teamAvatar("M", Color(.sRGB, red: 0.898, green: 0.541, blue: 0.420, opacity: 1))
                    .offset(x: 16)
            }
            .frame(width: 40, alignment: .leading)
            Text(plurals(SildKeys.Plural.widgetHomeAgentsOnline, teamOnline, [:]))
                .font(style.font(13))
                .foregroundStyle(style.colors.onBrand.opacity(0.9))
                .padding(.leading, 24)
        }
    }

    private func teamAvatar(_ letter: String, _ bg: Color) -> some View {
        Circle().fill(bg).frame(width: 24, height: 24)
            .overlay(Text(letter).font(style.font(11, .bold)).foregroundStyle(.white))
    }

    private var newConversationCard: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(t(SildKeys.widgetHomeCta))
                .font(style.font(15, .bold))
                .foregroundStyle(style.colors.text)
            Text(t(SildKeys.widgetHomeReassurance))
                .font(style.font(13))
                .foregroundStyle(style.colors.sub)
                .padding(.top, 4)
            Button(action: onNew) {
                HStack(spacing: 8) {
                    Text(t(SildKeys.widgetHomeNewConversation)).font(style.font(15, .bold))
                    SildIconView(.arrow, size: 16)
                }
                .foregroundStyle(style.colors.onBrand)
                .frame(maxWidth: .infinity)
                .padding(.vertical, 13)
            }
            .buttonStyle(SildBrandButtonStyle(colors: style.colors, radius: style.radii.btn))
            .padding(.top, 14)
            .accessibilityIdentifier("sild.home.new")
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(style.colors.card)
        .clipShape(RoundedRectangle(cornerRadius: style.radii.card))
    }

    private func topicRow(_ topic: String) -> some View {
        Button(action: onNew) {
            HStack {
                Text(topic).font(style.font(14)).foregroundStyle(style.colors.text)
                Spacer()
                SildIconView(.chevron, size: 14).foregroundStyle(style.colors.tertiary)
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 13)
            .background(style.colors.card)
            .clipShape(RoundedRectangle(cornerRadius: style.radii.btn))
        }
        .buttonStyle(.plain)
    }

    private var recentList: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(t(SildKeys.widgetHomeRecent))
                .font(style.font(11, .semibold))
                .foregroundStyle(style.colors.tertiary)
                .padding(.top, 4)
                .padding(.bottom, 8)
            VStack(spacing: 0) {
                ForEach(Array(state.conversations.enumerated()), id: \.element.id) { i, c in
                    if i > 0 {
                        Rectangle().fill(style.colors.border).frame(height: 1)
                    }
                    conversationRow(c)
                }
            }
            .background(style.colors.card)
            .clipShape(RoundedRectangle(cornerRadius: style.radii.card))
        }
    }

    private func conversationRow(_ c: Conversation) -> some View {
        let title = c.title ?? c.agentName ?? (state.agentName ?? t(SildKeys.widgetHomeSupport))
        return Button { onOpen(c.id) } label: {
            HStack(alignment: .top, spacing: 11) {
                SildAvatar(name: title, size: 40, background: style.colors.brand)
                VStack(alignment: .leading, spacing: 0) {
                    HStack {
                        Text(title)
                            .font(style.font(14, .semibold))
                            .foregroundStyle(style.colors.text)
                            .lineLimit(1)
                        Spacer()
                        if !c.time.isEmpty {
                            Text(c.time).font(style.font(11)).foregroundStyle(style.colors.tertiary)
                        }
                    }
                    Text(c.preview)
                        .font(style.font(13))
                        .foregroundStyle(style.colors.sub)
                        .lineLimit(1)
                        .padding(.top, 2)
                    if let sub = c.subtitle ?? (c.closed ? t(SildKeys.widgetThreadClosedShort) : nil), !sub.isEmpty {
                        Text(sub)
                            .font(style.font(11))
                            .foregroundStyle(style.colors.tertiary)
                            .lineLimit(1)
                            .padding(.top, 3)
                    }
                }
            }
            .padding(14)
        }
        .buttonStyle(.plain)
    }
}

/// One conversation (support or peer), or a draft support request not yet created. In
/// draft mode the header reads "Type your message to start" and the first send creates
/// the request, then sends — the same lazy-create the web widget does.
struct ThreadScreen: View {
    @Environment(\.sildStyle) private var style
    @Environment(\.openURL) private var openURL
    @Environment(\.sildStrings) private var strings

    private func t(_ key: String, _ vars: [String: Any] = [:]) -> String { strings(key, vars) }

    let model: SildModel
    let draft: Bool
    let onBack: () -> Void
    let onClose: () -> Void

    @State private var pending: [PendingAttachment] = []
    @State private var uploading = 0
    @State private var attachError: String?
    @State private var photoItems: [PhotosPickerItem] = []
    @State private var showAttachMenu = false
    @State private var showPhotoPicker = false
    @State private var showFileImporter = false

    private func title(_ active: Conversation?) -> String {
        active?.peer == true ? (active?.title ?? t(SildKeys.widgetHomeDirectChat)) : (state.agentName ?? t(SildKeys.widgetHomeSupport))
    }

    private func subtitle(_ active: Conversation?) -> String {
        if active?.peer == true { return active?.subtitle ?? t(SildKeys.widgetHomeDirectChat) }
        if draft { return t(SildKeys.widgetHomeStart) }
        return state.connection == .connected ? t(SildKeys.widgetHomeSubtitle) : t(SildKeys.widgetStatusConnecting)
    }

    private var state: SildState { model.state }

    var body: some View {
        let active = state.conversations.first { $0.id == state.activeId }
        VStack(spacing: 0) {
            SildHeader(
                title: title(active),
                subtitle: subtitle(active),
                onBack: onBack,
                soundOn: state.soundOn,
                onToggleSound: { model.toggleSound() },
                onClose: onClose
            )
            connectionBanner
            messages
            if active?.closed == true {
                Text(t(SildKeys.widgetThreadClosed))
                    .font(style.font(13))
                    .foregroundStyle(style.colors.sub)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(14)
                    .background(style.colors.card)
            }
            // Send/create/attachment failures surface here; the composer keeps the draft.
            if let problem = state.error ?? attachError {
                SildErrorLine(text: problem)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.horizontal, 14)
                    .padding(.vertical, 6)
                    .background(style.colors.card)
            }
            SildComposer(
                pending: pending,
                uploading: uploading,
                enabled: active?.closed != true,
                onAttach: { showAttachMenu = true },
                onRemove: { i in pending.remove(at: i) },
                onSend: { body in
                    let sent = pending
                    if body.isEmpty && sent.isEmpty { return false }
                    let ok = await model.send(body, attachments: sent)
                    // Drop only the attachments we actually sent — the picker stays live
                    // during the request, so any added meanwhile are kept.
                    if ok { pending.removeAll { att in sent.contains { $0.objectKey == att.objectKey } } }
                    return ok
                }
            )
        }
        .background(style.colors.page)
        .confirmationDialog("Attach", isPresented: $showAttachMenu, titleVisibility: .hidden) {
            Button(t(SildKeys.widgetComposerPhotoOrVideo)) { showPhotoPicker = true }
            Button(t(SildKeys.widgetComposerFile)) { showFileImporter = true }
            Button(t(SildKeys.widgetCommonCancel), role: .cancel) {}
        }
        .photosPicker(isPresented: $showPhotoPicker, selection: $photoItems, matching: .any(of: [.images, .videos]))
        .onChange(of: photoItems) { _, items in
            guard !items.isEmpty else { return }
            attachError = nil
            photoItems = []
            for item in items { attach(item) }
        }
        // Compose accepts */*, so the file importer covers everything the photo picker
        // does not.
        .fileImporter(isPresented: $showFileImporter, allowedContentTypes: [.item], allowsMultipleSelection: true) { result in
            attachError = nil
            guard case let .success(urls) = result else { return }
            for url in urls { attachFile(url) }
        }
    }

    // Realtime is what keeps a thread live, so say so when it is not up yet. The probe
    // carries the raw state for UI tests, which must gate on a real connection rather
    // than on REST timing.
    @ViewBuilder
    private var connectionBanner: some View {
        if state.connection != ConnectionState.connected {
            Text(t(state.connection == ConnectionState.disconnected ? SildKeys.widgetStatusReconnecting : SildKeys.widgetStatusConnecting))
                .font(style.font(12))
                .foregroundStyle(style.colors.sub)
                .frame(maxWidth: .infinity, alignment: .center)
                .padding(.vertical, 5)
                .background(style.colors.sunken)
        }
        Color.clear
            .frame(width: 1, height: 1)
            .accessibilityElement()
            .accessibilityIdentifier("sild.connection")
            .accessibilityValue(state.connection.name)
    }

    private var messages: some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 12) {
                    if state.loadingThread && state.messages.isEmpty {
                        Text(t(SildKeys.widgetStatusLoading)).font(style.font(13)).foregroundStyle(style.colors.tertiary)
                    }
                    ForEach(state.messages, id: \.id) { m in
                        SildMessageBubble(message: m) { url in
                            // Tenant-supplied data: any other scheme could take the user away.
                            guard let u = URL(string: url), let s = u.scheme?.lowercased(),
                                  s == "http" || s == "https" else { return }
                            openURL(u)
                        }
                        .id(m.id)
                    }
                    if !state.loadingThread && state.messages.isEmpty {
                        Text(t(SildKeys.widgetThreadEmpty))
                            .font(style.font(13))
                            .foregroundStyle(style.colors.tertiary)
                    }
                    Color.clear.frame(height: 4).id("sild.bottom")
                }
                .padding(.horizontal, 14)
                .padding(.vertical, 16)
            }
            .onChange(of: state.messages.count) { _, _ in
                withAnimation { proxy.scrollTo("sild.bottom", anchor: .bottom) }
            }
        }
    }

    /// Run one attachment upload, keeping the tray count and the error line in step.
    private func attaching(_ work: @escaping () async throws -> Void) {
        uploading += 1
        Task {
            defer { uploading -= 1 }
            do {
                try await work()
            } catch is SizeLimitExceeded {
                attachError = tooLargeMessage
            } catch {
                attachError = t(SildKeys.widgetComposerAttachFailed)
            }
        }
    }

    private func attach(_ item: PhotosPickerItem) {
        attaching {
            // loadTransferable would buffer the whole asset before we could check its
            // size; the file representation lets us measure first and skip a pick that
            // would not fit in memory anyway.
            guard let file = try await item.loadFileRepresentation() else { return }
            defer { try? FileManager.default.removeItem(at: file) }
            try await upload(file, fallbackName: "attachment")
        }
    }

    private func attachFile(_ url: URL) {
        attaching {
            let scoped = url.startAccessingSecurityScopedResource()
            defer { if scoped { url.stopAccessingSecurityScopedResource() } }
            try await upload(url, fallbackName: url.lastPathComponent)
        }
    }

    /// Check the size on disk before reading, so a multi-GB pick is rejected rather
    /// than buffered. A client-side memory bound, not the backend's per-tenant limit.
    private func upload(_ url: URL, fallbackName: String) async throws {
        let size = (try? url.resourceValues(forKeys: [.fileSizeKey]).fileSize) ?? 0
        guard Int64(size) <= model.uploadSizeLimitBytes else { throw SizeLimitExceeded() }
        let data = try Data(contentsOf: url, options: .mappedIfSafe)
        guard Int64(data.count) <= model.uploadSizeLimitBytes else { throw SizeLimitExceeded() }
        let name = url.lastPathComponent.isEmpty ? fallbackName : url.lastPathComponent
        let mime = UTType(filenameExtension: url.pathExtension)?.preferredMIMEType ?? "application/octet-stream"
        let att = try await model.upload(bytes: data, filename: name, mimeType: mime)
        pending.append(att)
    }

    private var tooLargeMessage: String {
        t(SildKeys.widgetComposerTooLarge, ["mb": model.uploadSizeLimitBytes / (1024 * 1024)])
    }
}

private struct SizeLimitExceeded: Error {}

private extension PhotosPickerItem {
    /// Copy the pick to a temporary file without loading it into memory.
    func loadFileRepresentation() async throws -> URL? {
        guard let transfer = try await loadTransferable(type: PickedFile.self) else { return nil }
        return transfer.url
    }
}

/// Receives the picked asset as a file on disk rather than as bytes.
private struct PickedFile: Transferable {
    let url: URL

    static var transferRepresentation: some TransferRepresentation {
        FileRepresentation(importedContentType: .item) { received in
            let copy = FileManager.default.temporaryDirectory
                .appendingPathComponent(UUID().uuidString)
                .appendingPathExtension(received.file.pathExtension)
            try FileManager.default.copyItem(at: received.file, to: copy)
            return PickedFile(url: copy)
        }
    }
}
