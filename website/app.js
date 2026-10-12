/**
 * LIFELOG Website Interactive Logic
 * Features:
 * - OS Auto-detection (macOS Apple Silicon, Intel, Windows, Linux)
 * - Multi-platform download dropdown & direct download links
 * - VS Code-style interactive screenshot viewer
 * - Interactive Terminal Playground with realistic LIFELOG CLI outputs
 * - Clipboard copy helper
 * - Scroll-driven reveal animations
 */

document.addEventListener('DOMContentLoaded', () => {
  initOsDetection();
  initDownloadDropdown();
  initShowcaseTabs();
  initTerminalTabs();
  initCopyButtons();
  initScrollAnimations();
  initFaqAccordion();
});

// OS Detection
function initOsDetection() {
  const userAgent = window.navigator.userAgent.toLowerCase();
  const platform = window.navigator.platform.toLowerCase();
  
  let os = 'mac-arm'; // Default to modern macOS
  let label = 'Download for macOS';
  let sub = 'Apple Silicon (M1/M2/M3/M4) • DMG';
  let href = 'assets/downloads/LifeLog-0.2.0-arm64.dmg';

  if (platform.includes('win') || userAgent.includes('windows')) {
    os = 'windows';
    label = 'Download for Windows';
    sub = 'Windows 11 / 10 (x64) • Setup.exe';
    href = '#download-platforms';
  } else if (platform.includes('linux') || userAgent.includes('linux')) {
    os = 'linux';
    label = 'Download for Linux';
    sub = 'Ubuntu / Debian (.deb) / AppImage';
    href = '#download-platforms';
  } else if (platform.includes('mac') || userAgent.includes('macintosh')) {
    // Check for Intel vs Apple Silicon
    const isIntel = userAgent.includes('intel') && !userAgent.includes('arm');
    if (isIntel && !navigator.maxTouchPoints) {
      os = 'mac-intel';
      label = 'Download for macOS (Intel)';
      sub = 'x86_64 DMG Installer';
      href = '#download-platforms';
    } else {
      os = 'mac-arm';
      label = 'Download for macOS';
      sub = 'Apple Silicon (M1/M2/M3/M4) • 117 MB';
      href = 'assets/downloads/LifeLog-0.2.0-arm64.dmg';
    }
  }

  const mainBtn = document.getElementById('heroDownloadBtn');
  const mainMeta = document.getElementById('heroDownloadMeta');
  if (mainBtn && mainMeta) {
    mainBtn.href = href;
    const labelSpan = mainBtn.querySelector('.download-label-text');
    if (labelSpan) labelSpan.textContent = label;
    mainMeta.textContent = sub;
  }
}

// Download Dropdown
function initDownloadDropdown() {
  const chevronBtn = document.getElementById('downloadChevronBtn');
  const dropdown = document.getElementById('downloadDropdown');

  if (chevronBtn && dropdown) {
    chevronBtn.addEventListener('click', (e) => {
      e.stopPropagation();
      dropdown.classList.toggle('show');
    });

    document.addEventListener('click', (e) => {
      if (!dropdown.contains(e.target) && e.target !== chevronBtn) {
        dropdown.classList.remove('show');
      }
    });
  }
}

// Interactive Screenshot Viewer (VS Code style)
function initShowcaseTabs() {
  const tabs = document.querySelectorAll('.tab-btn');
  const img = document.getElementById('showcaseImage');

  const screenshots = {
    'overview': 'assets/screenshots/screenshot-light-1200x800.png',
    'apps': 'assets/screenshots/screenshot-apps-light-1200x800.png',
    'dark': 'assets/screenshots/screenshot-dark-1200x800.png',
    'search': 'assets/screenshots/screenshot-dark-search-modal.png'
  };

  tabs.forEach(tab => {
    tab.addEventListener('click', () => {
      tabs.forEach(t => t.classList.remove('active'));
      tab.classList.add('active');

      const target = tab.getAttribute('data-tab');
      if (img && screenshots[target]) {
        img.style.opacity = '0.4';
        setTimeout(() => {
          img.src = screenshots[target];
          img.style.opacity = '1';
        }, 120);
      }
    });
  });
}

// Interactive Terminal Playground
function initTerminalTabs() {
  const termTabs = document.querySelectorAll('.term-tab');
  const termContent = document.getElementById('termOutput');

  const outputs = {
    'resume': `<div class="term-cmd">lifelog resume</div>
<div class="term-out">================================================================================
LIFELOG RESUMPTION REPORT — PICK UP WHERE YOU LEFT OFF
Generated: 2026-10-12 18:30:00 UTC | Inactivity Gap: 30m
================================================================================

--------------------------------------------------------------------------------
1. OBSERVED ACTIVITY (Ground Truth Evidence)
--------------------------------------------------------------------------------

[Filesystem Modifications]
  Tracked Directory: /Users/dakshsrivastava/Desktop/LifeLog
  Modified Files (12 events):
    2026-10-12 18:24:12  modified  11,223 B  internal/ipc/server.go
    2026-10-12 18:22:45  modified  23,304 B  internal/ipc/handlers.go
    2026-10-12 18:18:02  modified   5,257 B  desktop/src/main/index.ts

[Local Git Status]
  Repository ID:  repo_bf89e902c6e174e9
  Branch:         main
  Active Commits: feat(v2): implement daemon IPC, desktop shell, system activity
  Working Tree:   Clean

[Application Usage]
  Active App:     Visual Studio Code (com.microsoft.VSCode)
  Session Length: 42m 18s

--------------------------------------------------------------------------------
2. DERIVED ACTIVITY SESSIONS & SYNTHESIS (Computed Context)
--------------------------------------------------------------------------------
  Session 1: 17:42:00 -> 18:24:12 (42m, 24 events)
  Primary Focus: Implementing IPC token handshake security & Electron window isolation

--------------------------------------------------------------------------------
3. UNAVAILABLE CONTEXT & PRIVACY BOUNDARIES
--------------------------------------------------------------------------------
  - File contents and code diffs not inspected (privacy boundary).
  - Terminal output and shell history never recorded (privacy boundary).
================================================================================</div>`,

    'status': `<div class="term-cmd">lifelog status</div>
<div class="term-out">LIFELOG Daemon Status
======================
Operational State:  <span class="term-success">ACTIVE (RUNNING)</span>
PID:                48912
Unix Domain Socket: /Users/daksh/Library/Application Support/LifeLog/lifelog.sock
Socket Permissions: <span class="term-success">0600 (owner read/write only)</span>
Handshake Token:    Active (ephemeral cryptographically secure)
SQLite WAL DB:      /Users/daksh/Library/Application Support/LifeLog/lifelog.db
Schema Version:     <span class="term-success">v9 (forward-only migrations)</span>
Observed Observers:
  - Filesystem Poller: <span class="term-success">Running</span> (1 directory watched)
  - Native App Helper: <span class="term-success">Running</span> (com.lifelog.app-observer)
  - System Lifecycle:  <span class="term-success">Active</span> (Sleep/Wake/Lock/Unlock hooks)
  - Browser Domains:   <span class="term-warn">Disabled (opt-in boundary)</span></div>`,

    'events': `<div class="term-cmd">lifelog events --limit 5</div>
<div class="term-out">LIFELOG Filesystem Events (showing latest 5)
================================================================================
TIMESTAMP (UTC)           TYPE       SIZE (B)    PATH
2026-10-12 18:24:12 UTC   modified   11,223      internal/ipc/server.go
2026-10-12 18:22:45 UTC   modified   23,304      internal/ipc/handlers.go
2026-10-12 18:18:02 UTC   modified    5,257      desktop/src/main/index.ts
2026-10-12 18:15:10 UTC   modified    1,431 KB   desktop/assets/icon.icns
2026-10-12 18:10:04 UTC   created    28,530      desktop/dist/assets/index.css
================================================================================</div>`,

    'search': `<div class="term-cmd">lifelog search "token handshake"</div>
<div class="term-out">LIFELOG Unified Full-Text Search (FTS5 BM25 + Recency)
Query: "token handshake" (Execution: 0.84ms)
================================================================================
1. [FS] internal/ipc/handlers.go (Score: 4.82)
   "Constant-time token verification using subtle.ConstantTimeCompare"
   Timestamp: 2026-10-12 18:22:45 UTC

2. [TERM] lifelog daemon --socket=... (Score: 3.19)
   "Daemon process spawned with ephemeral handshake token passed in env"
   Timestamp: 2026-10-12 18:05:00 UTC

3. [SYS] system_lock transition (Score: 2.11)
   "Finalized active app sessions upon screen lock event"
   Timestamp: 2026-10-12 17:30:12 UTC
================================================================================</div>`
  };

  termTabs.forEach(tab => {
    tab.addEventListener('click', () => {
      termTabs.forEach(t => t.classList.remove('active'));
      tab.classList.add('active');

      const cmd = tab.getAttribute('data-cmd');
      if (termContent && outputs[cmd]) {
        termContent.innerHTML = outputs[cmd];
      }
    });
  });
}

// Copy to Clipboard
function initCopyButtons() {
  const copyBtns = document.querySelectorAll('.btn-copy');
  copyBtns.forEach(btn => {
    btn.addEventListener('click', () => {
      const text = btn.getAttribute('data-copy');
      if (text) {
        navigator.clipboard.writeText(text).then(() => {
          const originalHTML = btn.innerHTML;
          btn.innerHTML = `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#059669" stroke-width="2.5"><polyline points="20 6 9 17 4 12"></polyline></svg>`;
          setTimeout(() => {
            btn.innerHTML = originalHTML;
          }, 1800);
        });
      }
    });
  });
}

// Scroll-driven Reveal Animations
function initScrollAnimations() {
  const observer = new IntersectionObserver((entries) => {
    entries.forEach(entry => {
      if (entry.isIntersecting) {
        entry.target.classList.add('revealed');
      }
    });
  }, {
    threshold: 0.1
  });

  document.querySelectorAll('.pipeline-step, .feature-card, .comparison-card, .platform-card').forEach(el => {
    el.style.opacity = '0';
    el.style.transform = 'translateY(16px)';
    el.style.transition = 'opacity 0.4s ease, transform 0.4s ease';
    observer.observe(el);
  });
}

// FAQ Accordion
function initFaqAccordion() {
  const faqItems = document.querySelectorAll('.faq-item');
  faqItems.forEach((item, index) => {
    if (index === 0) item.classList.add('open');
    const question = item.querySelector('.faq-question');
    if (question) {
      question.addEventListener('click', () => {
        item.classList.toggle('open');
      });
    }
  });
}

// Reveal class injection helper
const style = document.createElement('style');
style.textContent = `
  .revealed {
    opacity: 1 !important;
    transform: translateY(0) !important;
  }
`;
document.head.appendChild(style);
