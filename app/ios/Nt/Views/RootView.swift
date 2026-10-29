// RootView is the first run until the notes are cloned, then the notes: one stack on an iPhone
// (StackLayout), the sidebar beside the note on an iPad or a wide window (SplitLayout).
import QuickLook
import SwiftUI

struct RootView: View {
    @Environment(Phone.self) private var phone

    var body: some View {
        Group {
            if let ws = phone.workspace {
                WorkspaceView()
                    .environment(ws)
            } else if phone.cloned {
                starting
            } else {
                SetupView()
            }
        }
        // on screen it syncs, also when a Shortcut started nt with the app off screen
        .task { await phone.open() }
    }

    private var starting: some View {
        VStack(spacing: 14) {
            if let p = phone.problem {
                Text(p)
                    .foregroundStyle(Brand.ink)
                    .multilineTextAlignment(.center)
                    .textSelection(.enabled)
                Button("Try again") { Task { await phone.open() } }
            } else {
                ProgressView()
            }
        }
        .padding()
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(Brand.ground)
    }
}

struct WorkspaceView: View {
    @Environment(Workspace.self) private var ws
    @Environment(\.horizontalSizeClass) private var size

    var body: some View {
        Group {
            switch ws.phase {
            case .loading:
                ProgressView()
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            case .problem(let message):
                VStack(spacing: 14) {
                    Text(message)
                        .foregroundStyle(Brand.ink)
                        .multilineTextAlignment(.center)
                        .textSelection(.enabled)
                    Button("Try again") { Task { await ws.start() } }
                }
                .padding()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            case .ready:
                // the sidebar beside the note on an iPad with room for it; a stack on an iPhone,
                // upright or on its side, and on an iPad in a narrow window
                if UIDevice.current.userInterfaceIdiom == .pad && size == .regular {
                    SplitLayout()
                } else {
                    StackLayout()
                }
            }
        }
        .background(Brand.ground)
        .tint(Brand.accent)
        .overlay(alignment: .bottom) { Flash() }
        .modifier(Asking())
        .quickLookPreview(Binding(get: { ws.preview }, set: { ws.preview = $0 }))
    }
}

/// Flash is the one line the app has for the person: what went wrong, or where a link pointed.
struct Flash: View {
    @Environment(Workspace.self) private var ws

    var body: some View {
        Group {
            if !ws.flash.isEmpty {
                Text(ws.flash)
                    .font(.footnote)
                    .foregroundStyle(Brand.ink)
                    .padding(.horizontal, 14)
                    .padding(.vertical, 8)
                    .background(Brand.surface, in: Capsule())
                    .overlay(Capsule().stroke(Brand.line))
                    .padding(.horizontal, 16)
                    .padding(.bottom, 72)
                    .transition(.opacity)
            }
        }
        .animation(.easeOut(duration: 0.2), value: ws.flash)
    }
}
