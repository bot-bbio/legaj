import json
import sys
import os
import random

def generate_anki_deck(cards_data, output_path, deck_name):
    try:
        import genanki
    except ImportError:
        print("Error: genanki is not installed.")
        sys.exit(1)
        
    # Generate unique IDs for model and deck
    model_id = random.randrange(1 << 30, 1 << 31)
    deck_id = random.randrange(1 << 30, 1 << 31)
    
    # Define simple Q&A model
    anki_model = genanki.Model(
        model_id,
        'LeGaJ Interview Prep Model',
        fields=[
            {'name': 'Question'},
            {'name': 'Answer'},
        ],
        templates=[
            {
                'name': 'Card 1',
                'qfmt': '<div style="font-family: Arial; font-size: 20px; text-align: center; padding: 20px;">{{Question}}</div>',
                'afmt': '{{FrontSide}}<hr id="answer"><div style="font-family: Arial; font-size: 16px; text-align: left; padding: 10px; line-height: 1.4;">{{Answer}}</div>',
            },
        ]
    )
    
    deck = genanki.Deck(deck_id, deck_name)
    
    for item in cards_data:
        question = item.get("question", "")
        answer = item.get("answer", "")
        
        # Replace newlines with <br> for HTML rendering in Anki
        answer_html = answer.replace("\n", "<br>")
        
        note = genanki.Note(
            model=anki_model,
            fields=[question, answer_html]
        )
        deck.add_note(note)
        
    os.makedirs(os.path.dirname(output_path), exist_ok=True)
    genanki.Package(deck).write_to_file(output_path)
    print(f"Successfully generated Anki deck ({len(cards_data)} cards) at: {output_path}")

def generate_cheatsheet(cheatsheet_data, output_path):
    try:
        with open(output_path, 'w', encoding='utf-8') as f:
            f.write(f"# Interview Cheatsheet: {cheatsheet_data.get('role', 'Target Role')} at {cheatsheet_data.get('company', 'Target Company')}\n\n")
            
            f.write("## 1. Company Profile & Mission\n")
            f.write(f"{cheatsheet_data.get('company_profile', 'N/A')}\n\n")
            
            f.write("## 2. Core Elevator Pitch\n")
            f.write(f"{cheatsheet_data.get('elevator_pitch', 'N/A')}\n\n")
            
            f.write("## 3. Top Achievements to Reference\n")
            for ach in cheatsheet_data.get("key_achievements", []):
                f.write(f"- {ach}\n")
            f.write("\n")
            
            f.write("## 4. Tough Questions & Strategy Answers\n")
            for idx, qa in enumerate(cheatsheet_data.get("questions_and_answers", []), 1):
                f.write(f"### Q{idx}: {qa.get('question')}\n")
                f.write(f"**Answer Strategy:** {qa.get('answer')}\n\n")
                
            f.write("## 5. Smart Questions to Ask the Interviewer\n")
            for q in cheatsheet_data.get("questions_to_ask", []):
                f.write(f"- {q}\n")
                
        print(f"Successfully generated interview cheatsheet at: {output_path}")
    except Exception as e:
        print(f"Error writing cheatsheet: {str(e)}")

def main():
    if len(sys.argv) < 3:
        print("Usage: python prepare_interview.py <prep_data_json> <mode: anki|cheatsheet|all> [output_path]")
        sys.exit(1)
        
    data_path = sys.argv[1]
    mode = sys.argv[2].lower()
    
    if not os.path.exists(data_path):
        print(f"Error: File '{data_path}' not found.")
        sys.exit(1)
        
    with open(data_path, 'r', encoding='utf-8') as f:
        data = json.load(f)
        
    default_anki_path = "outputs/interview_deck.apkg"
    default_sheet_path = "outputs/interview_cheatsheet.md"
    
    company = data.get("company", "Target_Company").replace(" ", "_")
    role = data.get("role", "Target_Role").replace(" ", "_")
    
    if mode in ['anki', 'all']:
        anki_out = sys.argv[3] if len(sys.argv) > 3 and mode == 'anki' else f"outputs/{company}_{role}_interview_deck.apkg"
        cards = data.get("flashcards", [])
        if cards:
            generate_anki_deck(cards, anki_out, f"{data.get('company')} - {data.get('role')} Prep")
        else:
            print("No flashcards found in source JSON.")
            
    if mode in ['cheatsheet', 'all']:
        sheet_out = sys.argv[3] if len(sys.argv) > 3 and mode == 'cheatsheet' else f"outputs/{company}_{role}_interview_cheatsheet.md"
        generate_cheatsheet(data, sheet_out)

if __name__ == "__main__":
    main()
