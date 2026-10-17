
const { ethers } = require("hardhat");

async function main() {
  const recipient = process.env.BAJ_RECIPIENT;

  if (!recipient || !ethers.isAddress(recipient)) {
    throw new Error("Enter a valid recipient address");
  }

  const [sender] = await ethers.getSigners();

  const tokenAddress =
    "0x5FbDB2315678afecb367f032d93F642f64180aa3";

  const code = await ethers.provider.getCode(tokenAddress);

  if (code === "0x") {
    throw new Error("Token not deployed on current node");
  }

  const token = await ethers.getContractAt(
    "BajarToken",
    tokenAddress,
    sender
  );

  const amount = ethers.parseUnits("1000", 9);

  const tx = await token.transfer(recipient, amount);
  await tx.wait();

  const gasTx = await sender.sendTransaction({
    to: recipient,
    value: ethers.parseEther("0.1")
  });
  await gasTx.wait();

  console.log("Sent 1000 BAJ");
  console.log("Sent 0.1 local test ETH for gas");
  console.log("Recipient:", recipient);
  console.log("Transaction:", tx.hash);
  console.log(
    "BAJ balance:",
    ethers.formatUnits(
      await token.balanceOf(recipient),
      9
    )
  );
}

main().catch(console.error);
