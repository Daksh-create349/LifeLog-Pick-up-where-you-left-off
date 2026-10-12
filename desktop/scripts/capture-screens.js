const { app, BrowserWindow } = require('electron');
const path = require('path');
const fs = require('fs');
const { DaemonSupervisor } = require('../dist-electron/main/daemon.js');
const { setupIpcBridge } = require('../dist-electron/main/bridge.js');

let mainWindow = null;
let supervisor = null;

const screenshotsDir = path.resolve(__dirname, '../screenshots');
if (!fs.existsSync(screenshotsDir)) {
  fs.mkdirSync(screenshotsDir, { recursive: true });
}

app.whenReady().then(async () => {
  console.log('App ready, initializing supervisor...');
  supervisor = new DaemonSupervisor();
  setupIpcBridge(supervisor, () => mainWindow);

  mainWindow = new BrowserWindow({
    width: 1200,
    height: 800,
    show: false,
    backgroundColor: '#FBFBFA',
    webPreferences: {
      preload: path.resolve(__dirname, '../dist-electron/preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  });

  const htmlPath = path.resolve(__dirname, '../dist/index.html');
  await mainWindow.loadFile(htmlPath);
  console.log('Loaded HTML file into window');

  // Start Go daemon
  await supervisor.start();
  console.log('Supervisor started, waiting for data rendering...');

  // Wait 4 seconds for full data loading and rendering
  await new Promise((r) => setTimeout(r, 4000));

  // 1. Capture Normal Size (1200x800) Light Mode
  const imgLight = await mainWindow.webContents.capturePage();
  const lightPath = path.join(screenshotsDir, 'screenshot-light-1200x800.png');
  fs.writeFileSync(lightPath, imgLight.toPNG());
  console.log(`Saved screenshot: ${lightPath} (${fs.statSync(lightPath).size} bytes)`);

  // 2. Switch to Dark Mode & Capture
  await mainWindow.webContents.executeJavaScript(`
    document.documentElement.classList.add('dark');
  `);
  await new Promise((r) => setTimeout(r, 600));

  const imgDark = await mainWindow.webContents.capturePage();
  const darkPath = path.join(screenshotsDir, 'screenshot-dark-1200x800.png');
  fs.writeFileSync(darkPath, imgDark.toPNG());
  console.log(`Saved screenshot: ${darkPath} (${fs.statSync(darkPath).size} bytes)`);

  // 3. Resize to Compact Window (960x640) Dark Mode
  mainWindow.setSize(960, 640);
  await new Promise((r) => setTimeout(r, 600));

  const imgCompact = await mainWindow.webContents.capturePage();
  const compactPath = path.join(screenshotsDir, 'screenshot-dark-960x640.png');
  fs.writeFileSync(compactPath, imgCompact.toPNG());
  console.log(`Saved screenshot: ${compactPath} (${fs.statSync(compactPath).size} bytes)`);

  // 4. Open Cmd+K Search Palette & Capture
  await mainWindow.webContents.executeJavaScript(`
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', metaKey: true }));
  `);
  await new Promise((r) => setTimeout(r, 600));

  const imgSearch = await mainWindow.webContents.capturePage();
  const searchPath = path.join(screenshotsDir, 'screenshot-dark-search-modal.png');
  fs.writeFileSync(searchPath, imgSearch.toPNG());
  console.log(`Saved screenshot: ${searchPath} (${fs.statSync(searchPath).size} bytes)`);

  // Close search modal
  await mainWindow.webContents.executeJavaScript(`
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
  `);
  await new Promise((r) => setTimeout(r, 400));

  // Reset to 1200x800 for Apps view
  mainWindow.setSize(1200, 800);
  await new Promise((r) => setTimeout(r, 400));

  // 5. Navigate to Apps View (Dark Mode)
  await mainWindow.webContents.executeJavaScript(`
    const btn = Array.from(document.querySelectorAll('button')).find(b => b.textContent && b.textContent.includes('Apps'));
    if (btn) btn.click();
  `);
  await new Promise((r) => setTimeout(r, 1500));

  const imgAppsDark = await mainWindow.webContents.capturePage();
  const appsDarkPath = path.join(screenshotsDir, 'screenshot-apps-dark-1200x800.png');
  fs.writeFileSync(appsDarkPath, imgAppsDark.toPNG());
  console.log(`Saved screenshot: ${appsDarkPath} (${fs.statSync(appsDarkPath).size} bytes)`);

  // 6. Switch Apps View to Light Mode
  await mainWindow.webContents.executeJavaScript(`
    document.documentElement.classList.remove('dark');
  `);
  await new Promise((r) => setTimeout(r, 600));

  const imgAppsLight = await mainWindow.webContents.capturePage();
  const appsLightPath = path.join(screenshotsDir, 'screenshot-apps-light-1200x800.png');
  fs.writeFileSync(appsLightPath, imgAppsLight.toPNG());
  console.log(`Saved screenshot: ${appsLightPath} (${fs.statSync(appsLightPath).size} bytes)`);

  // 7. Compact Apps View (960x640) Light Mode
  mainWindow.setSize(960, 640);
  await new Promise((r) => setTimeout(r, 600));

  const imgAppsCompact = await mainWindow.webContents.capturePage();
  const appsCompactPath = path.join(screenshotsDir, 'screenshot-apps-compact-960x640.png');
  fs.writeFileSync(appsCompactPath, imgAppsCompact.toPNG());
  console.log(`Saved screenshot: ${appsCompactPath} (${fs.statSync(appsCompactPath).size} bytes)`);

  // Clean shutdown
  console.log('Visual verification complete. Shutting down...');
  await supervisor.stop();
  app.quit();
});
