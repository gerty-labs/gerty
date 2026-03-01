# Data Sources

This document tracks the provenance and licensing of all training data sources.

## Source Registry

| Source | Licence | Attribution Required | Collection Script | Status |
|--------|---------|---------------------|-------------------|--------|
| K8s official docs | Apache 2.0 | Yes (but permissive) | `collect_k8s_docs.py` | Scaffold |
| GKE/EKS/AKS docs | Proprietary | Fair use for training | `collect_k8s_docs.py` | Scaffold |
| GitHub issues | Varies by repo | Check per-repo | `collect_gh_issues.py` | Scaffold |
| Stack Overflow | CC BY-SA 4.0 | Yes (URL in provenance) | `collect_so.py` | Scaffold |
| VPA recommender source | Apache 2.0 | Yes | Manual extraction | Not started |
| Goldilocks source | Apache 2.0 | Yes | Manual extraction | Not started |
| Expert knowledge | Original (proprietary) | N/A | Hand-written | 20 seed pairs |
| Synthetic generation | Original (proprietary) | N/A | `generate_synthetic.py` | Not started |

## Licensing Notes

- **Apache 2.0**: Permissive. Can use for training. Must acknowledge source.
- **CC BY-SA 4.0** (Stack Overflow): Must attribute. ShareAlike means derived works must use compatible licence. Training data itself inherits CC BY-SA; model outputs do not (model is a transformation, not a derived work under most interpretations).
- **Proprietary docs** (cloud providers): Transformation into instruction pairs for model training is considered fair use. Do not reproduce verbatim paragraphs.
- **GitHub issues**: Public discussions. Check the repository licence for any specific restrictions. Most K8s-related repos are Apache 2.0.
