#!/usr/bin/env python3
"""Merge LoRA adapter into base model and optionally convert to GGUF.

Steps:
1. Load base model + LoRA adapter
2. Merge adapter weights into base model
3. Save merged model in HuggingFace format
4. (Optional) Convert to GGUF via llama.cpp's convert_hf_to_gguf.py

Usage:
    python merge_and_quantize.py \
        --adapter-path output/k8s-sage-lora \
        --output-dir output/k8s-sage-merged

    # With GGUF conversion (requires llama.cpp checkout):
    python merge_and_quantize.py \
        --adapter-path output/k8s-sage-lora \
        --output-dir output/k8s-sage-merged \
        --gguf \
        --llama-cpp-path /path/to/llama.cpp
"""

import argparse
import logging
import shutil
import subprocess
import sys
from pathlib import Path

logger = logging.getLogger(__name__)


def merge_adapter(adapter_path: str, output_dir: str) -> None:
    """Load base model + LoRA adapter, merge, and save."""
    # Load adapter config to get base model name
    from peft import PeftConfig, PeftModel
    from transformers import AutoModelForCausalLM, AutoTokenizer

    peft_config = PeftConfig.from_pretrained(adapter_path)
    base_model_name = peft_config.base_model_name_or_path
    logger.info("Base model: %s", base_model_name)
    logger.info("Adapter: %s", adapter_path)

    logger.info("Loading base model (full precision for merge)...")
    model = AutoModelForCausalLM.from_pretrained(
        base_model_name,
        trust_remote_code=True,
    )

    tokenizer = AutoTokenizer.from_pretrained(base_model_name, trust_remote_code=True)

    logger.info("Loading LoRA adapter...")
    model = PeftModel.from_pretrained(model, adapter_path)

    logger.info("Merging adapter into base model...")
    model = model.merge_and_unload()

    logger.info("Saving merged model to %s", output_dir)
    Path(output_dir).mkdir(parents=True, exist_ok=True)
    model.save_pretrained(output_dir)
    tokenizer.save_pretrained(output_dir)

    logger.info("Merged model saved successfully.")


def convert_to_gguf(model_dir: str, llama_cpp_path: str, quantization: str = "q4_k_m") -> None:
    """Convert merged HuggingFace model to GGUF format."""
    convert_script = Path(llama_cpp_path) / "convert_hf_to_gguf.py"
    if not convert_script.exists():
        logger.error("convert_hf_to_gguf.py not found at %s", convert_script)
        logger.info("To convert manually, run:")
        logger.info("  python %s %s --outtype %s", convert_script, model_dir, quantization)
        return

    output_file = Path(model_dir) / f"k8s-sage-{quantization}.gguf"

    cmd = [
        sys.executable,
        str(convert_script),
        model_dir,
        "--outtype",
        quantization,
        "--outfile",
        str(output_file),
    ]

    logger.info("Converting to GGUF: %s", " ".join(cmd))
    result = subprocess.run(cmd, capture_output=True, text=True)

    if result.returncode != 0:
        logger.error("GGUF conversion failed:\n%s", result.stderr)
        sys.exit(1)

    logger.info("GGUF model saved to %s", output_file)
    logger.info("File size: %.1f MB", output_file.stat().st_size / (1024 * 1024))


def main() -> None:
    parser = argparse.ArgumentParser(description="Merge LoRA adapter and optionally quantize to GGUF")
    parser.add_argument("--adapter-path", default="output/k8s-sage-lora", help="Path to LoRA adapter directory")
    parser.add_argument("--output-dir", default="output/k8s-sage-merged", help="Output directory for merged model")
    parser.add_argument("--gguf", action="store_true", help="Convert to GGUF format after merging")
    parser.add_argument("--llama-cpp-path", default="", help="Path to llama.cpp repo (for GGUF conversion)")
    parser.add_argument("--quantization", default="q4_k_m", help="GGUF quantization type (default: q4_k_m)")
    args = parser.parse_args()

    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")

    merge_adapter(args.adapter_path, args.output_dir)

    if args.gguf:
        if not args.llama_cpp_path:
            llama_cpp = shutil.which("llama-server")
            if llama_cpp:
                args.llama_cpp_path = str(Path(llama_cpp).parent.parent)
            else:
                logger.warning("--llama-cpp-path not set and llama-server not found on PATH.")
                logger.info("To convert to GGUF, clone llama.cpp and pass --llama-cpp-path.")
                logger.info("  git clone https://github.com/ggerganov/llama.cpp")
                logger.info("  python merge_and_quantize.py --gguf --llama-cpp-path ./llama.cpp")
                return

        convert_to_gguf(args.output_dir, args.llama_cpp_path, args.quantization)


if __name__ == "__main__":
    main()
