const fs = require("node:fs");
const path = require("node:path");

const nodeInstallRoot = path.dirname(
  path.dirname(path.dirname(process.execPath)),
);
const nodeInstallations = fs
  .readdirSync(nodeInstallRoot, { withFileTypes: true })
  .filter(
    (entry) =>
      entry.isDirectory() &&
      fs.existsSync(path.join(nodeInstallRoot, entry.name, "bin", "node")),
  )
  .map((entry) => entry.name)
  .sort();

console.log(`Node.js version: ${process.version}`);
console.log(`Node.js installation count: ${nodeInstallations.length}`);
console.log(`Node.js installations: ${nodeInstallations.join(", ")}`);
