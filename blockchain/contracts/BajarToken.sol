// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "@openzeppelin/contracts/token/ERC20/ERC20.sol";

contract BajarToken is ERC20 {
    uint256 public constant MAX_SUPPLY =
        1_000_000_000_000 * 10 ** 9;

    constructor(address initialHolder)
        ERC20("BAJAR", "BAJ")
    {
        require(
            initialHolder != address(0),
            "Invalid holder"
        );

        _mint(initialHolder, MAX_SUPPLY);
    }

    function decimals()
        public
        pure
        override
        returns (uint8)
    {
        return 9;
    }
}
