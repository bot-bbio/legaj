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
        
    doc = SimpleDocTemplate(output_path, pagesize=letter,
                            rightMargin=72, leftMargin=72,  # 1 inch margins
                            topMargin=72, bottomMargin=72)
    
    styles = getSampleStyleSheet()
    
    # Header Styles
    header_name_style = ParagraphStyle(
        'HeaderName',
        parent=styles['Normal'],
        fontName='Times-Bold',
        fontSize=12,
        alignment=1, # Center
        spaceAfter=2
    )
    
    header_info_style = ParagraphStyle(
        'HeaderInfo',
        parent=styles['Normal'],
        fontName='Times-Roman',
        fontSize=11,
        alignment=1, # Center
        spaceAfter=12
    )
    
    # Body Style (Times-Roman, 11pt, Double-spaced 22pt leading)
    body_style = ParagraphStyle(
        'BodyStyle',
        parent=styles['Normal'],
        fontName='Times-Roman',
        fontSize=11,
        leading=22, # exactly 22pt leading
        alignment=0, # Left
        spaceAfter=12
    )
    
    recipient_style = ParagraphStyle(
        'RecipientStyle',
        parent=styles['Normal'],
        fontName='Times-Roman',
        fontSize=11,
        leading=14,
        alignment=0,
        spaceAfter=2
    )
    
    elements = []
    
    # Parse User Info for Header
    info = profile.get("personal_info", {})
    name = info.get("name", "Roberto Montero")
    
    contact_parts = []
    if info.get("email"): contact_parts.append(info["email"])
    if info.get("phone"): contact_parts.append(info["phone"])
    if info.get("location"): contact_parts.append(info["location"])
    
    contact_str = " | ".join(contact_parts)
    
    # Add Header
    elements.append(Paragraph(name, header_name_style))
    elements.append(Paragraph(contact_str, header_info_style))
    elements.append(Spacer(1, 10))
    
    # Parse draft text to separate recipient block/date from body paragraphs
    raw_lines = draft_text.split('\n')
    lines = [line.strip() for line in raw_lines]
    
    in_body = False
    current_para_lines = []
    
    for line in lines:
        if line == "":
            if current_para_lines:
                para_text = " ".join(current_para_lines)
                if in_body:
                    # Check if this is the signature/name
                    if para_text == name or para_text == "Roberto Montero":
                        elements.append(Spacer(1, 22)) # two blank lines before name
                        elements.append(Paragraph(para_text, body_style))
                    else:
                        elements.append(Paragraph(para_text, body_style))
                else:
                    # If we haven't hit the salutation yet, each line in this paragraph is rendered separately
                    # to keep formatting of address block
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
        
    if current_para_lines:
        para_text = " ".join(current_para_lines)
        if in_body:
            if para_text == name or para_text == "Roberto Montero":
                elements.append(Spacer(1, 22))
                elements.append(Paragraph(para_text, body_style))
            else:
                elements.append(Paragraph(para_text, body_style))
        else:
            for l in current_para_lines:
                elements.append(Paragraph(l, recipient_style))
                
    doc.build(elements)
    print(f"Successfully generated Cover Letter PDF: {output_path}")
    
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
