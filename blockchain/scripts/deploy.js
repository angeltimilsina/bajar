const { ethers } = require("hardhat");

async function main() {
  const [deployer] = await ethers.getSigners();

  console.log("Deploying BAJAR Token...");
  console.log("Deployer:", deployer.address);

  const BajarToken =
    await ethers.getContractFactory("BajarToken");

  const token = await BajarToken.deploy(
    deployer.address
  );

  await token.waitForDeployment();

  const address = await token.getAddress();
  const supply = await token.totalSupply();

  console.log("--------------------------");
  console.log("BAJAR DEPLOYED SUCCESSFULLY");
  console.log("--------------------------");
  console.log("Contract:", address);
  console.log("Name:", await token.name());
  console.log("Symbol:", await token.symbol());
  console.log("Decimals:", await token.decimals());
  console.log(
    "Total Supply:",
    ethers.formatUnits(supply, 9),
    "BAJ"
  );
  console.log("Network: BAJAR Local");
  console.log("Chain ID: 1337");
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
