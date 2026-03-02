#!/usr/bin/env python3
"""QLoRA fine-tuning script for k8s-sage SLM.

Uses HuggingFace TRL SFTTrainer with PEFT LoRA adapters and
bitsandbytes 4-bit quantization. Config loaded from YAML.

Usage:
    # Dry run (no GPU needed, validates config + dataset):
    python finetune_lora.py --dry-run

    # Full training:
    python finetune_lora.py --config configs/default.yaml

    # Without WandB logging:
    python finetune_lora.py --no-wandb
"""

import argparse
import json
import logging
from pathlib import Path

import yaml

logger = logging.getLogger(__name__)


def load_config(path: str) -> dict:
    """Load training config from YAML file."""
    with open(path) as f:
        config = yaml.safe_load(f)

    required_keys = ["base_model", "output_dir", "lora", "quantization", "training", "data"]
    for key in required_keys:
        if key not in config:
            raise ValueError(f"Missing required config key: {key}")

    return config


def load_dataset(config: dict) -> "Dataset":
    """Load and split the training dataset from JSONL."""
    from datasets import Dataset

    train_file = config["data"]["train_file"]
    if not Path(train_file).exists():
        raise FileNotFoundError(f"Training data not found: {train_file}")

    records = []
    with open(train_file) as f:
        for line in f:
            line = line.strip()
            if line:
                records.append(json.loads(line))

    dataset = Dataset.from_list(records)
    logger.info("Loaded %d training examples from %s", len(dataset), train_file)
    return dataset


def format_chat(example: dict, tokenizer: "PreTrainedTokenizer") -> dict:
    """Convert a training example to chat-template format.

    Each example has {system, user, assistant} fields.
    Uses the tokenizer's chat template to produce the final text.
    """
    messages = [
        {"role": "system", "content": example["system"]},
        {"role": "user", "content": example["user"]},
        {"role": "assistant", "content": example["assistant"]},
    ]

    text = tokenizer.apply_chat_template(messages, tokenize=False, add_generation_prompt=False)
    return {"text": text}


def setup_model_and_tokenizer(config: dict) -> tuple:
    """Load base model with QLoRA quantization and prepare LoRA adapter."""
    import torch
    from peft import LoraConfig, get_peft_model, prepare_model_for_kbit_training
    from transformers import AutoModelForCausalLM, AutoTokenizer, BitsAndBytesConfig

    quant_config = config["quantization"]
    compute_dtype = getattr(torch, quant_config.get("bnb_4bit_compute_dtype", "bfloat16"))

    bnb_config = BitsAndBytesConfig(
        load_in_4bit=quant_config.get("load_in_4bit", True),
        bnb_4bit_quant_type=quant_config.get("bnb_4bit_quant_type", "nf4"),
        bnb_4bit_compute_dtype=compute_dtype,
        bnb_4bit_use_double_quant=True,
    )

    logger.info("Loading base model: %s", config["base_model"])
    model = AutoModelForCausalLM.from_pretrained(  # nosec B615 — model from controlled config
        config["base_model"],
        quantization_config=bnb_config,
        device_map="auto",
        trust_remote_code=True,
    )

    tokenizer = AutoTokenizer.from_pretrained(  # nosec B615 — model from controlled config
        config["base_model"],
        trust_remote_code=True,
    )
    if tokenizer.pad_token is None:
        tokenizer.pad_token = tokenizer.eos_token

    model = prepare_model_for_kbit_training(model)

    lora_cfg = config["lora"]
    peft_config = LoraConfig(
        r=lora_cfg["r"],
        lora_alpha=lora_cfg["alpha"],
        lora_dropout=lora_cfg["dropout"],
        target_modules=lora_cfg["target_modules"],
        bias="none",
        task_type="CAUSAL_LM",
    )

    model = get_peft_model(model, peft_config)

    trainable, total = 0, 0
    for param in model.parameters():
        total += param.numel()
        if param.requires_grad:
            trainable += param.numel()
    logger.info("Parameters: %d total, %d trainable (%.2f%%)", total, trainable, 100 * trainable / total)

    return model, tokenizer


def print_dry_run_summary(config: dict, dataset: "Dataset") -> None:
    """Print a summary of what training would do, without loading the model."""
    print("\n=== DRY RUN SUMMARY ===\n")
    print(f"Base model:        {config['base_model']}")
    print(f"Output dir:        {config['output_dir']}")
    print(f"Dataset size:      {len(dataset)} examples")

    eval_split = config["data"].get("eval_split", 0.1)
    train_size = int(len(dataset) * (1 - eval_split))
    eval_size = len(dataset) - train_size
    print(f"Train/eval split:  {train_size}/{eval_size} ({eval_split:.0%} eval)")

    lora = config["lora"]
    print("\nLoRA config:")
    print(f"  Rank:            {lora['r']}")
    print(f"  Alpha:           {lora['alpha']}")
    print(f"  Dropout:         {lora['dropout']}")
    print(f"  Target modules:  {', '.join(lora['target_modules'])}")

    t = config["training"]
    eff_batch = t["per_device_train_batch_size"] * t["gradient_accumulation_steps"]
    steps_per_epoch = train_size // eff_batch
    total_steps = steps_per_epoch * t["num_train_epochs"]
    print("\nTraining config:")
    print(f"  Epochs:          {t['num_train_epochs']}")
    print(f"  Batch size:      {t['per_device_train_batch_size']} × {t['gradient_accumulation_steps']} = {eff_batch}")
    print(f"  Learning rate:   {t['learning_rate']}")
    print(f"  Max seq length:  {t['max_seq_length']}")
    print(f"  Est. steps:      ~{total_steps} ({steps_per_epoch}/epoch)")

    # Validate a sample
    sample = dataset[0]
    print(f"\nSample example ID: {sample.get('id', 'N/A')}")
    print(f"  System:          {sample.get('system', '')[:80]}...")
    print(f"  User:            {sample.get('user', '')[:80]}...")
    print(f"  Assistant:       {sample.get('assistant', '')[:80]}...")

    # Check source distribution
    sources: dict[str, int] = {}
    for ex in dataset:
        src = ex.get("source", "unknown")
        sources[src] = sources.get(src, 0) + 1
    print("\nSource distribution:")
    for src, count in sorted(sources.items()):
        print(f"  {src}: {count}")

    print("\n=== DRY RUN COMPLETE — no GPU resources used ===")


def main() -> None:
    parser = argparse.ArgumentParser(description="QLoRA fine-tuning for k8s-sage SLM")
    parser.add_argument("--config", default="ml/training/configs/default.yaml", help="Path to training config YAML")
    parser.add_argument("--dry-run", action="store_true", help="Validate config and dataset without loading model")
    parser.add_argument("--no-wandb", action="store_true", help="Disable WandB logging")
    args = parser.parse_args()

    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(message)s",
    )

    config = load_config(args.config)

    if args.no_wandb:
        import os

        os.environ["WANDB_DISABLED"] = "true"

    dataset = load_dataset(config)

    if args.dry_run:
        print_dry_run_summary(config, dataset)
        return

    model, tokenizer = setup_model_and_tokenizer(config)

    # Format dataset with chat template
    formatted = dataset.map(lambda ex: format_chat(ex, tokenizer), remove_columns=dataset.column_names)

    # Train/eval split
    eval_split = config["data"].get("eval_split", 0.1)
    split = formatted.train_test_split(test_size=eval_split, seed=42)
    train_dataset = split["train"]
    eval_dataset = split["test"]
    logger.info("Train: %d examples, Eval: %d examples", len(train_dataset), len(eval_dataset))

    # Setup trainer
    from transformers import TrainingArguments
    from trl import SFTTrainer

    t = config["training"]
    training_args = TrainingArguments(
        output_dir=config["output_dir"],
        learning_rate=t["learning_rate"],
        lr_scheduler_type=t.get("lr_scheduler_type", "cosine"),
        warmup_steps=t.get("warmup_steps", 100),
        num_train_epochs=t["num_train_epochs"],
        per_device_train_batch_size=t["per_device_train_batch_size"],
        gradient_accumulation_steps=t.get("gradient_accumulation_steps", 4),
        bf16=t.get("bf16", True),
        weight_decay=t.get("weight_decay", 0.01),
        eval_strategy=t.get("eval_strategy", "steps"),
        eval_steps=t.get("eval_steps", 100),
        save_strategy=t.get("save_strategy", "steps"),
        save_steps=t.get("save_steps", 200),
        logging_steps=t.get("logging_steps", 10),
        report_to="wandb" if not args.no_wandb else "none",
        remove_unused_columns=False,
    )

    trainer = SFTTrainer(
        model=model,
        tokenizer=tokenizer,
        args=training_args,
        train_dataset=train_dataset,
        eval_dataset=eval_dataset,
        max_seq_length=t.get("max_seq_length", 2048),
        dataset_text_field="text",
    )

    logger.info("Starting training...")
    trainer.train()

    # Save LoRA adapter
    output_dir = config["output_dir"]
    logger.info("Saving LoRA adapter to %s", output_dir)
    trainer.save_model(output_dir)
    tokenizer.save_pretrained(output_dir)

    logger.info("Training complete.")


if __name__ == "__main__":
    main()
