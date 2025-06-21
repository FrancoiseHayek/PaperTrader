# sweep.py
import subprocess
import json
import itertools
import pandas as pd
from tabulate import tabulate
from tqdm import tqdm

# 1) Define your grid of lists
# param_grid = {
#   "Notional": ["10", "50", "100", "200", "250", "500"],
#   "RSI_thresh": [20, 25, 30, 35, 40],
#   "RSI_window": [5, 10, 15, 20, 25],
#   "VolMA_thresh": [0.8, 0.9, 1.0, 1.1, 1.2],
#   "VolMA_window": [5, 7, 10, 15, 17, 20],
#   "Profit_factor": [0.0005, 0.001, 0.002, 0.005]
# }

param_grid = {
  "Notional": ["100", "10"],
  "RSI_thresh": [35, 25],
  "RSI_window": [25, 20],
  "VolMA_thresh": [1.1, 1.2],
  "VolMA_window": [17, 20],
  "Profit_factor": [0.005, 0.02]
}

# 2) Build the list of all combinations
keys = list(param_grid.keys())
all_combos = [
    dict(zip(keys, values))
    for values in itertools.product(*(param_grid[k] for k in keys))
]

def run_harness(params, symbol, start, end):
    # overwrite the JSON your algorithm reads
    with open("../algorithms/rsi_bullish_parameters.json", "w") as f:
        json.dump(params, f)
    with open("../algorithms/rsi_bullish_state.json", "w") as f:
        json.dump({"Cash": 1000, "OpenPositions": 0, "SharesHeld": 0, "SharesWon": 0.0}, f)
    # call the Go program
    subprocess.run([
        "go", "run", "./harness.go",
        "--symbol", symbol,
        "--start", start,
        "--end", end
    ], check=True)
    # read back the results JSON
    res_file = f"results_{symbol}_{start}_{end}.json"
    return pd.read_json(res_file)

def main():
    symbol, start, end = "SPY", "2024-01-01", "2024-12-31"
    summary = []

    total = len(all_combos)
    for params in tqdm(all_combos, total=total):
      res = run_harness(params, symbol, start, end)

      # Extract the pieces you need
      state = res["State"]    
      n_open = state["OpenPositions"]
      avg_pos = res["AverageNumPositions"].iloc[0]
      n_trades = res["NumTrades"].iloc[0]       

      # Merge params and these metrics into one dict
      entry = {
          **params,                            # expands your grid parameters
          "csh_end":        state["Cash"],    # pick whichever fields you want from state
          "opn_pos":  n_open,
          "shrs_held":     state["SharesHeld"],
          "avg_pos":   avg_pos,
          "num_trds":      n_trades,
      }
      summary.append(entry)

    df_summary = pd.DataFrame(summary)

    sorted_df = df_summary.sort_values("csh_end", ascending=False)

    pd.set_option('display.max_columns', None)
    pd.set_option('display.width', 120)  # or use shutil.get_terminal_size()
    pd.set_option('display.max_colwidth', None)

    print(tabulate(sorted_df, headers='keys', tablefmt='github'))


if __name__ == "__main__":
    main()
