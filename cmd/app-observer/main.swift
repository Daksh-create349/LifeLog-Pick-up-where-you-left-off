import Foundation
import AppKit

// Protocol constants
let protocolVersion = "1.0"

// Thread-safe stdout writer with automatic flushing
func emitJSON(_ dict: [String: Any]) {
    guard let data = try? JSONSerialization.data(withJSONObject: dict, options: []),
          let str = String(data: data, encoding: .utf8) else {
        return
    }
    print(str)
    fflush(stdout)
}

func iso8601Timestamp(_ date: Date = Date()) -> String {
    let formatter = ISO8601DateFormatter()
    formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    return formatter.string(from: date)
}

func sanitizeAppName(_ name: String?) -> String {
    guard let name = name?.trimmingCharacters(in: .whitespacesAndNewlines), !name.isEmpty else {
        return "Unknown Application"
    }
    return name
}

func sanitizeBundleID(_ bundleID: String?, fallbackName: String) -> String {
    guard let bid = bundleID?.trimmingCharacters(in: .whitespacesAndNewlines), !bid.isEmpty else {
        return fallbackName.lowercased().replacingOccurrences(of: " ", with: ".")
    }
    return bid
}

class AppObserver {
    private var lastBundleID: String = ""
    private var lastAppName: String = ""
    private var heartbeatTimer: Timer?
    private var isSleeping: Bool = false
    private var isLocked: Bool = false

    func start() {
        // 1. Initial foreground application detection
        reportCurrentForeground(reason: "app_activated")

        // 2. Register for AppKit workspace notifications
        let center = NSWorkspace.shared.notificationCenter

        center.addObserver(
            self,
            selector: #selector(handleAppActivated(_:)),
            name: NSWorkspace.didActivateApplicationNotification,
            object: nil
        )

        center.addObserver(
            self,
            selector: #selector(handleSystemSleep(_:)),
            name: NSWorkspace.willSleepNotification,
            object: nil
        )

        center.addObserver(
            self,
            selector: #selector(handleSystemWake(_:)),
            name: NSWorkspace.didWakeNotification,
            object: nil
        )

        // Distributed notification center for screen lock and unlock
        let distCenter = DistributedNotificationCenter.default()
        distCenter.addObserver(
            self,
            selector: #selector(handleScreenLocked(_:)),
            name: NSNotification.Name("com.apple.screenIsLocked"),
            object: nil
        )
        distCenter.addObserver(
            self,
            selector: #selector(handleScreenUnlocked(_:)),
            name: NSNotification.Name("com.apple.screenIsUnlocked"),
            object: nil
        )

        // 3. Periodic heartbeat timer (every 15 seconds)
        heartbeatTimer = Timer.scheduledTimer(withTimeInterval: 15.0, repeats: true) { [weak self] _ in
            self?.sendHeartbeat()
        }

        // 4. Stdin monitoring: exit immediately when parent process closes pipe
        setupStdinWatcher()
    }

    @objc private func handleAppActivated(_ notification: Notification) {
        guard !isSleeping, !isLocked else { return }

        guard let app = notification.userInfo?[NSWorkspace.applicationUserInfoKey] as? NSRunningApplication else {
            reportCurrentForeground(reason: "app_activated")
            return
        }

        let appName = sanitizeAppName(app.localizedName)
        let bundleID = sanitizeBundleID(app.bundleIdentifier, fallbackName: appName)

        // Deduplicate redundant notifications for the exact same active app
        if bundleID == lastBundleID && appName == lastAppName {
            return
        }

        lastBundleID = bundleID
        lastAppName = appName

        emitJSON([
            "version": protocolVersion,
            "event": "app_activated",
            "bundle_id": bundleID,
            "app_name": appName,
            "pid": app.processIdentifier,
            "timestamp": iso8601Timestamp()
        ])
    }

    @objc private func handleSystemSleep(_ notification: Notification) {
        isSleeping = true
        emitJSON([
            "version": protocolVersion,
            "event": "system_sleep",
            "timestamp": iso8601Timestamp()
        ])
    }

    @objc private func handleSystemWake(_ notification: Notification) {
        isSleeping = false
        emitJSON([
            "version": protocolVersion,
            "event": "system_wake",
            "timestamp": iso8601Timestamp()
        ])
        // Re-report foreground app upon waking
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.2) { [weak self] in
            guard let self = self, !self.isLocked else { return }
            self.reportCurrentForeground(reason: "app_activated")
        }
    }

    @objc private func handleScreenLocked(_ notification: Notification) {
        isLocked = true
        emitJSON([
            "version": protocolVersion,
            "event": "system_lock",
            "timestamp": iso8601Timestamp()
        ])
    }

    @objc private func handleScreenUnlocked(_ notification: Notification) {
        isLocked = false
        emitJSON([
            "version": protocolVersion,
            "event": "system_unlock",
            "timestamp": iso8601Timestamp()
        ])
        // Re-report foreground app upon unlocking
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.2) { [weak self] in
            guard let self = self, !self.isSleeping else { return }
            self.reportCurrentForeground(reason: "app_activated")
        }
    }

    private func sendHeartbeat() {
        guard !isSleeping, !isLocked, !lastBundleID.isEmpty else { return }
        emitJSON([
            "version": protocolVersion,
            "event": "heartbeat",
            "bundle_id": lastBundleID,
            "app_name": lastAppName,
            "timestamp": iso8601Timestamp()
        ])
    }

    private func reportCurrentForeground(reason: String) {
        guard let app = NSWorkspace.shared.frontmostApplication else {
            return
        }
        let appName = sanitizeAppName(app.localizedName)
        let bundleID = sanitizeBundleID(app.bundleIdentifier, fallbackName: appName)

        lastBundleID = bundleID
        lastAppName = appName

        emitJSON([
            "version": protocolVersion,
            "event": reason,
            "bundle_id": bundleID,
            "app_name": appName,
            "pid": app.processIdentifier,
            "timestamp": iso8601Timestamp()
        ])
    }

    private func setupStdinWatcher() {
        // Read stdin on background dispatch source. If EOF (length == 0), exit cleanly.
        let stdinHandle = FileHandle.standardInput
        stdinHandle.readabilityHandler = { handle in
            let data = handle.availableData
            if data.isEmpty {
                // Pipe closed by parent Go daemon
                exit(0)
            }
        }
    }
}

// Signal handling
signal(SIGINT) { _ in exit(0) }
signal(SIGTERM) { _ in exit(0) }
signal(SIGPIPE, SIG_IGN)

// Start observer
let observer = AppObserver()
observer.start()

// Run main event loop for AppKit notifications
RunLoop.main.run()
