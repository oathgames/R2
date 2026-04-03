const { app, BrowserWindow } = require('electron');
const fs = require('fs');
const path = require('path');

const size = parseInt(process.argv[2] || '256');

app.whenReady().then(async () => {
  const win = new BrowserWindow({
    show: false, width: size, height: size,
    backgroundColor: '#08080a',
    frame: false,
    useContentSize: true,
  });

  await win.loadFile(path.join(__dirname, 'icon-template.html'));
  await new Promise(r => setTimeout(r, 2000));

  const image = await win.webContents.capturePage();
  const png = image.toPNG();
  const filename = `icon-${size}.png`;
  fs.writeFileSync(path.join(__dirname, filename), png);
  console.log(`${filename}: ${png.length} bytes`);
  app.quit();
});
