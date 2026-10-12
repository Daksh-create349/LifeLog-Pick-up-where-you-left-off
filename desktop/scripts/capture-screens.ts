import { app, BrowserWindow } from 'electron';
import path from 'path';
import fs from 'fs';
import { DaemonSupervisor } from '../src/main/daemon';
import { setupIpcBridge } from '../src/main/bridge';

let mainWindow: BrowserWindow | null = null;
let supervisor: DaemonSupervisor | null = null;

const screenshotsDir = path.resolve(__dirname, '../screenshots');
if (!fs.existsSync(screenshotsDir)) {
  fs.mkdirSync(screenshotsDir, { recursive: true });
}

app.whenReady().then(async () => {
  supervisor = new DaemonSupervisor();
  setupIpcBridge(supervisor, () => mainWindow);

  mainWindow = new BrowserWindow({
    width: 1200,
    height: 800,
    show: false, // Offscreen or hidden
    webPreferences: {
      preload: path.resolve(__dirname, '../dist-electron/preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  });

  const htmlPath = path.resolve(__dirname, '../dist/index.html');
  await mainWindow.loadFile(htmlPath);

  // Start Go daemon
  await supervisor.start();

  // Wait 3 seconds for data loading and rendering
  await new Promise((r) => setTimeout(r, 3000));

  // 1. Capture Normal Size (1200x800) Light Mode
  const imgLight = await mainWindow.webContents.capturePage();
  const lightPath = path.join(screenshotsDir, 'screenshot-light-1200x800.png');
  fs.writeFileSync(lightPath, imgLight.toPNG());
  console.log(`Saved screenshot: ${lightPath}`);

  // 2. Switch to Dark Mode & Capture
  await mainWindow.webContents.executeJavaScript(`
    document.documentElement.classList.add('dark');
  `);
  await new Promise((r) => setTimeout(r, 600));

  const imgDark = await mainWindow.webContents.capturePage();
  const darkPath = path.join(screenshotsDir, 'screenshot-dark-1200x800.png');
  fs.writeFileSync(darkPath, imgDark.toPNG());
  console.log(`Saved screenshot: ${darkPath}`);

  // 3. Resize to Compact Window (960x640) Dark Mode
  mainWindow.setSize(960, 640);
  await new Promise((r) => setTimeout(r, 600));

  const imgCompact = await mainWindow.webContents.capturePage();
  const compactPath = path.join(screenshotsDir, 'screenshot-dark-960x640.png');
  fs.writeFileSync(compactPath, imgCompact.toPNG());
  console.log(`Saved screenshot: ${compactPath}`);

  // Clean shutdown
  console.log('Shutting down supervisor...');
  await supervisor.stop();
  app.quit();
});
