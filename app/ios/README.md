# nt for iPhone and iPad

The notes repository on the phone: a full clone, synced with git over ssh, written by nt's own
Go code running inside the app. A third client of the same repository; docs/design.md §9 is the
spec, and phase 7 lists its steps, all built: the first run makes the phone's key and clones the
notes; then the home list (Today, Todos, Calendar, Weekly, Agents, the pinned folders and the
folders, search, the sync state), notes in live preview that are written as on the Mac, the
views, notes and folders made, renamed, moved and deleted, the capture sheet and Settings,
voice, and Shortcuts and Siri. It ships through TestFlight (`make ios-testflight`), not the App
Store yet; `make ios-device` puts it on a connected phone.

## Build

Needs Xcode 26 or later, [XcodeGen](https://github.com/yonaskolb/XcodeGen), Go with gomobile
(`go install golang.org/x/mobile/cmd/gomobile@latest golang.org/x/mobile/cmd/gobind@latest`)
and cmake (`brew install cmake`).

```sh
make ios-build      # the core and libgit2 when missing, the project, a simulator build
make ios-test       # the Swift tests in the simulator
make ios-ssh-test   # ssh with the phone's key, against a throwaway sshd on 127.0.0.1
make ios-voice      # speech to words in English, Dutch and Slovak, on the Mac (the simulator has no models)
make ios-device     # build, install and launch on a connected iPhone or iPad
make ios-testflight # archive and upload to App Store Connect, for TestFlight
make ios-core       # rebuild nt for the phone after a change to the Go code
```

`make ios-core` builds the `mobile` package into `build/NtCore.xcframework` with gomobile;
`make ios-libgit2` (`scripts/ios-libgit2.sh`) builds libgit2 1.9 with ssh through libssh2 on
mbedTLS, from source tarballs whose SHA-256 it checks, into `build/Git2.xcframework`. Neither is
committed. `make ios-project` writes an empty `Local.xcconfig`; put your team id in it
(`Local.xcconfig.example`) for a device build, and an App Store Connect API key for TestFlight.
Simulator builds are signed ad hoc, so the Keychain works there.

TestFlight needs the app's record in App Store Connect once (Apps › New App, iOS, bundle id
`nl.w4j.nt.ios`); `make ios-testflight` then archives a Release build, exports it with
`ExportOptions.plist` (Xcode raises the build number) and uploads it. Export compliance is in
the Info.plist (`ITSAppUsesNonExemptEncryption`, NO), as declared in App Store Connect: standard
encryption algorithms (ssh through libssh2 and mbedTLS), not on the App Store in France.

## The first run

1. The notes repository's ssh address, `git@github.com:<you>/notes.git`, and this device's name
   (`iphone`): the host in commits, conflict copies and reports.
2. **Make a key.** The phone makes an ECDSA P-256 key; the private half stays in its Keychain,
   on this device only. Copy the public half and add it on GitHub: the notes repository ›
   Settings › Deploy keys › Add deploy key, with **Allow write access** ticked.
3. **Clone.** GitHub's host keys are pinned; another host is trusted the first time and refused
   if its key changes (`known_hosts.json`).

The clone, nt's config and its state live in the app's Application Support. Commits from the
phone are made by `<host> <<host>@nt.invalid>`, unless the clone's git config names someone.

## Writing and sync

A note is written in place, and saved through nt as on the Mac (`nt _editor save`): a second
after the typing stops, a save that lands only on the version it was read from, the edit hold
while unsaved, a conflict copy when both changed the same line. The bar above the keyboard has a
task box, a heading, a link, a photo (from the library or the camera, saved beside the note as a
JPEG), the mic, undo, redo and done. The pencil at the foot of the home list adds a line to today, or a
todo. + on the home list and in a folder makes a note or a folder there; a long press on a row,
or a note's ⋯, renames, moves or deletes it (nt rename, mv and rm, which keep the links); a
swipe to the right moves or renames, to the left deletes, as in Mail. The phone syncs when it opens, half a minute after it wrote, when you leave it (a background
task lets it finish), when you come back to it, and in background refresh when iOS allows.

## Voice

The keyboard bar's mic dictates at the cursor: each piece goes in once the model has settled it,
the words it may still change shown faintly there; a tap again, or letting the keyboard go,
stops it, and holding it chooses the language, whose code shows under it. The mic in the capture
sheet talks a note instead: it listens until stopped, and each pause adds what was said as a
line to today's Log (`nt a`). The language, English, Nederlands or Slovenčina, is remembered on
the device. Apple's SpeechAnalyzer does the work on the device: SpeechTranscriber for English,
DictationTranscriber, the keyboard's model, for Dutch and Slovak; iOS downloads a language's
model the first time it is used. Only the words are kept, never the sound. The simulator has no
speech models, so `make ios-voice` runs the same Listener on the Mac, on a sentence in each
language that `say` speaks.

## Shortcuts and Siri

Two App Intents, "Add to today" and "New todo", take a line (Siri asks for it; `due:fri` sets a
todo's date) and write it as the capture sheet does, in the app's process without opening it.
App Shortcuts offer both to Siri ("New todo in nt"), Spotlight and the Action button with
nothing to set up. With the app off screen the phone syncs at once, in a background task.

## The screens

On an iPhone, one stack: the home list, then a folder, a note or a view, each a screen of its
own. A link followed in a note is a new screen, so Back returns; the one web view moves to the
screen on top. On an iPad, or in a window as wide, the sidebar beside the note, as on the Mac.
Pull the list down, or tap the sync state, to sync. For screenshots, `-NTView todos|calendar|
weekly|agents|settings`, `-NTFolder <path>` and `-NTOpen <path>` open a screen at launch, and
`-NTCapture today` the capture sheet; `assets/demo/phone.sh` takes the README's phone pictures
that way, in the simulator on the demo notes.

## Layout

```
project.yml          the Xcode project, generated by XcodeGen (the .xcodeproj is not committed)
Nt/NtApp.swift       the window: the first run, or the notes
Nt/Phone.swift       the first run (the address, the device's name, the key, the clone), then nt
                     on the clone and the shared Workspace
Nt/Core.swift        nt in the process (NtCore) and where it keeps things (Place)
Nt/Git/LibGit.swift  the repository's plumbing on libgit2, as nt's sync asks for it (mobile.Git)
Nt/Git/Remote.swift  ssh: the key libgit2 signs in with, and the host keys it trusts
Nt/Git/DeployKey.swift  the phone's key: CryptoKit, the Keychain, OpenSSH's formats
Nt/Views/            the first run; the home list and search; the two layouts (the iPhone's
                     stack with its folder and note screens, the iPad's split view); the
                     keyboard bar and photos; the capture sheet; Settings and the key; the mark
Nt/Voice/            speech to words (Listener, Mic), dictation at the cursor, Talk a note
Nt/Shortcuts.swift   the App Intents, Add to today and New todo, and their App Shortcuts
NtTests/             Swift Testing: LibGit against repositories on disk with nt's sync deciding,
                     the key's formats, ssh against a test sshd (make ios-ssh-test), where a
                     pause ends a spoken line, a Shortcut's line and todo through the phone's nt
```

`../shared` is compiled in with the Mac app: Brand, the model (Workspace, NT, RepoIndex, Capture,
…), the editor's web view and the Todos, Calendar and Agents views. The editor page is the Mac app's
(`app/macos/Nt/Resources/editor`, `make mac-editor`), with the design system's fonts.

## Third-party code

libgit2 (GPLv2 with the linking exception), libssh2 (BSD) and Mbed TLS (Apache 2.0) are linked
into the app from `build/Git2.xcframework`; the Go modules nt uses, from `build/NtCore.xcframework`.
