import json
import sys

def compare_profiles(base_path, tailored_path):
    try:
        with open(base_path, 'r', encoding='utf-8') as f:
            base = json.load(f)
        with open(tailored_path, 'r', encoding='utf-8') as f:
            tailored = json.load(f)
    except Exception as e:
        print(f"Error loading files: {str(e)}")
        sys.exit(1)
        
    print("\n" + "="*50)
    print(" RESUME TAILORING DIFF REPORT")
    print("="*50 + "\n")
    
    # Compare experience bullets
    base_exp = base.get("experience", [])
    tailored_exp = tailored.get("experience", [])
    
    for i, (b_job, t_job) in enumerate(zip(base_exp, tailored_exp)):
        company = b_job.get("company", f"Job {i+1}")
        role = b_job.get("role", "")
        print(f"[{role} at {company}]")
        
        b_bullets = b_job.get("bullets", [])
        t_bullets = t_job.get("bullets", [])
        
        for b_idx, (bb, tb) in enumerate(zip(b_bullets, t_bullets), 1):
            if bb.strip() != tb.strip():
                print(f"  Bullet {b_idx} Modified:")
                print(f"    - Original: {bb}")
                print(f"    + Tailored: {tb}")
                print()
        print("-" * 50)
        
    # Compare targeted skills if modified
    base_skills = base.get("skills", {})
    tailored_skills = tailored.get("skills", {})
    
    skills_diff = False
    for cat, b_list in base_skills.items():
        t_list = tailored_skills.get(cat, [])
        added = set(t_list) - set(b_list)
        removed = set(b_list) - set(t_list)
        if added or removed:
            if not skills_diff:
                print("\n[Skills Updates]")
                skills_diff = True
            print(f"  Category: {cat}")
            if added:
                print(f"    + Added: {', '.join(added)}")
            if removed:
                print(f"    - Removed: {', '.join(removed)}")
                
    if not skills_diff and not any(any(b != t for b, t in zip(bj.get("bullets", []), tj.get("bullets", []))) for bj, tj in zip(base_exp, tailored_exp)):
        print("No changes detected between base and tailored profiles.")

if __name__ == "__main__":
    if len(sys.argv) < 3:
        print("Usage: python tailor_resume.py <base_profile.json> <tailored_profile.json>")
        sys.exit(1)
    compare_profiles(sys.argv[1], sys.argv[2])
