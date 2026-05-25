import sys
import os
import json
import shutil
from datetime import datetime
from reportlab.lib.pagesizes import letter
from reportlab.lib import colors
from reportlab.lib.units import inch
from reportlab.lib.styles import getSampleStyleSheet, ParagraphStyle
from reportlab.platypus import SimpleDocTemplate, Paragraph, Spacer

def create_cover_letter_pdf(profile_path, draft_text_path, output_path):
    try:
        with open(profile_path, 'r', encoding='utf-8') as f:
            profile = json.load(f)
    except Exception as e:
        print(f"Error loading profile: {str(e)}")
        sys.exit(1)
        
    try:
        with open(draft_text_path, 'r', encoding='utf-8') as f:
            draft_text = f.read()
    except Exception as e:
        print(f"Error loading draft text: {str(e)}")
        sys.exit(1)
        

def generate_cl_elements(profile, draft_text, name, contact_str, pr):
    styles = getSampleStyleSheet()
    
    # Header Styles
    header_name_style = ParagraphStyle(
        'HeaderName',
        parent=styles['Normal'],
        fontName='Times-Bold',
        fontSize=pr["name_size"],
        alignment=1, # Center
        spaceAfter=2
    )
    
    header_info_style = ParagraphStyle(
        'HeaderInfo',
        parent=styles['Normal'],
        fontName='Times-Roman',
        fontSize=pr["info_size"],
        alignment=1, # Center
        spaceAfter=12
    )
    
    # Body Style
    body_style = ParagraphStyle(
        'BodyStyle',
        parent=styles['Normal'],
        fontName='Times-Roman',
        fontSize=pr["base_size"],
        leading=pr["leading"],
        alignment=0, # Left
        spaceAfter=pr["space_after"]
    )
    
    recipient_style = ParagraphStyle(
        'RecipientStyle',
        parent=styles['Normal'],
        fontName='Times-Roman',
        fontSize=pr["base_size"],
        leading=pr["base_size"] * 1.2,
        alignment=0,
        spaceAfter=2
    )
    
    elements = []
    
    # Add Header
    elements.append(Paragraph(name, header_name_style))
    elements.append(Paragraph(contact_str, header_info_style))
    elements.append(Spacer(1, 10))
    
    # Parse draft text to separate recipient block/date from body paragraphs
    raw_lines = draft_text.split('\n')
    lines = [line.strip() for line in raw_lines]
    
    in_body = False
    seen_sincerely = False
    current_para_lines = []
    
    for line in lines:
        if line == "":
            if current_para_lines:
                para_text = " ".join(current_para_lines)
                if in_body:
                    if seen_sincerely:
                        current_para_lines = []
                        continue
                    
                    lower_para = para_text.lower()
                    if "sincerely" in lower_para or "best regards" in lower_para or "warm regards" in lower_para or "respectfully" in lower_para:
                        elements.append(Paragraph(para_text, body_style))
                        elements.append(Spacer(1, 22))
                        elements.append(Paragraph(name, body_style))
                        seen_sincerely = True
                    else:
                        elements.append(Paragraph(para_text, body_style))
                else:
                    for l in current_para_lines:
                        elements.append(Paragraph(l, recipient_style))
                    elements.append(Spacer(1, 6))
                current_para_lines = []
            continue
            
        # Check if line is the salutation
        lower_line = line.lower()
        if not in_body and (lower_line.startswith("to whom") or lower_line.startswith("dear") or lower_line.startswith("hello")):
            if current_para_lines:
                for l in current_para_lines:
                    elements.append(Paragraph(l, recipient_style))
                elements.append(Spacer(1, 6))
                current_para_lines = []
            in_body = True
            elements.append(Paragraph(line, body_style))
            continue
            
        current_para_lines.append(line)
        
    if current_para_lines and not seen_sincerely:
        para_text = " ".join(current_para_lines)
        if in_body:
            lower_para = para_text.lower()
            if "sincerely" in lower_para or "best regards" in lower_para or "warm regards" in lower_para or "respectfully" in lower_para:
                elements.append(Paragraph(para_text, body_style))
                elements.append(Spacer(1, 22))
                elements.append(Paragraph(name, body_style))
            else:
                elements.append(Paragraph(para_text, body_style))
        else:
            for l in current_para_lines:
                elements.append(Paragraph(l, recipient_style))
                
    return elements

def create_cover_letter_pdf(profile_path, draft_text_path, output_path):
    try:
        with open(profile_path, 'r', encoding='utf-8') as f:
            profile = json.load(f)
    except Exception as e:
        print(f"Error loading profile: {str(e)}")
        sys.exit(1)
        
    try:
        with open(draft_text_path, 'r', encoding='utf-8') as f:
            draft_text = f.read()
    except Exception as e:
        print(f"Error loading draft text: {str(e)}")
        sys.exit(1)

    info = profile.get("personal_info", {})
    name = info.get("name", "[Full Name]")
    
    contact_parts = []
    if info.get("email"): contact_parts.append(info["email"])
    if info.get("phone"): contact_parts.append(info["phone"])
    if info.get("location"): contact_parts.append(info["location"])
    contact_str = " | ".join(contact_parts)

    presets = [
        {"base_size": 11, "leading": 14.5, "name_size": 13, "info_size": 10, "margin": 72, "space_after": 12},  # Standard
        {"base_size": 10.5, "leading": 14, "name_size": 12.5, "info_size": 9.5, "margin": 60, "space_after": 10},  # Compact
        {"base_size": 10, "leading": 13.5, "name_size": 12, "info_size": 9, "margin": 50, "space_after": 8},  # Compressed
        {"base_size": 9.5, "leading": 12.5, "name_size": 11.5, "info_size": 8.5, "margin": 40, "space_after": 6},  # Max Compressed
    ]

    success = False
    for idx, pr in enumerate(presets):
        doc = SimpleDocTemplate(output_path, pagesize=letter,
                                rightMargin=pr["margin"], leftMargin=pr["margin"],
                                topMargin=pr["margin"], bottomMargin=pr["margin"])
        
        elements = generate_cl_elements(profile, draft_text, name, contact_str, pr)
        doc.build(elements)
        
        if doc.page == 1:
            print(f"Successfully generated 1-page Cover Letter PDF at: {output_path} (preset {idx+1})")
            success = True
            break

    if not success:
        print(f"Warning: Cover Letter exceeded 1 page ({doc.page} pages) even with maximum visual lock compression.")
        
    # Save a copy to G Drive if available
    gdrive_dir = r"G:\My Drive\Personal Labour Mobile\Cover Letter PDFs\AI Cover Letters"
    if os.path.exists(gdrive_dir):
        # Extract company name from the output filename
        base_name = os.path.basename(output_path)
        dest_path = os.path.join(gdrive_dir, base_name)
        try:
            shutil.copy(output_path, dest_path)
            print(f"Successfully backed up Cover Letter PDF to G Drive: {dest_path}")
        except Exception as e:
            print(f"Warning: Could not copy Cover Letter PDF to G Drive: {str(e)}")

if __name__ == "__main__":
    if len(sys.argv) < 4:
        print("Usage: python generate_cover_letter_pdf.py <profile_json> <draft_text_file> <output_pdf>")
        sys.exit(1)
        
    profile_json = sys.argv[1]
    draft_text_file = sys.argv[2]
    output_pdf = sys.argv[3]
    
    create_cover_letter_pdf(profile_json, draft_text_file, output_pdf)
