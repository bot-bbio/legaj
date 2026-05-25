import os
import json
import sys

def create_tracker(file_path):
    if file_path.endswith(".xlsx"):
        file_path = file_path[:-5] + ".json"

    if os.path.exists(file_path):
        print(f"File already exists: {file_path}")
        return
    
    # Add a sample row with placeholders or instructions
    sample_data = [
        {
            "company": "Example Corp", 
            "role": "Product Manager", 
            "location": "New York, NY", 
            "date": "2026-05-22", 
            "link": "https://example.com/job", 
            "status": "Applied", 
            "resume": "Example_Corp_Resume_Tailored.pdf", 
            "cover_letter": "Example_Corp_Cover_Letter.pdf", 
            "notes": "Initial application sent. Follow up in 2 weeks."
        }
    ]
    
    os.makedirs(os.path.dirname(file_path), exist_ok=True)
    try:
        with open(file_path, 'w', encoding='utf-8') as f:
            json.dump(sample_data, f, indent=2)
        print(f"Successfully created JSON tracker at: {file_path}")
    except Exception as e:
        print(f"Error creating tracker: {e}")

if __name__ == "__main__":
    target = sys.argv[1] if len(sys.argv) > 1 else "references/job-tracker.json"
    create_tracker(target)
